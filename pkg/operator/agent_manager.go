package operator

import (
	"fmt"

	"github.com/blang/semver"
	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	v1meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"

	"github.com/coreos/container-linux-update-operator/pkg/constants"
	"github.com/coreos/container-linux-update-operator/pkg/k8sutil"
	"github.com/coreos/container-linux-update-operator/pkg/version"
)

var (
	daemonsetName = "container-linux-update-agent-ds"

	managedByOperatorLabels = map[string]string{
		"managed-by": "container-linux-update-operator",
		"app":        agentDefaultAppName,
	}

	// Labels nodes where update-agent should be scheduled
	enableUpdateAgentLabel = map[string]string{
		constants.LabelUpdateAgentEnabled: constants.True,
	}

	// Label Requirement matching nodes which lack the update agent label
	updateAgentLabelMissing = k8sutil.NewRequirementOrDie(
		constants.LabelUpdateAgentEnabled,
		selection.DoesNotExist,
		[]string{},
	)
)

// legacyLabeler finds Container Linux nodes lacking the update-agent enabled
// label and adds the label set "true" so nodes opt-in to running update-agent.
//
// Important: This behavior supports clusters which may have nodes that do not
// have labels which an update-agent daemonset might node select upon. Even if
// all current nodes are labeled, auto-scaling groups may create nodes lacking
// the label. Retain this behavior to support upgrades of Tectonic clusters
// created at 1.6.
func (k *Kontroller) legacyLabeler() {
	glog.V(6).Infof("Starting Container Linux node auto-labeler")

	nodelist, err := k.nc.List(v1meta.ListOptions{})
	if err != nil {
		glog.Infof("Failed listing nodes %v", err)
		return
	}

	// match nodes that don't have an update-agent label
	nodesMissingLabel := k8sutil.FilterNodesByRequirement(nodelist.Items, updateAgentLabelMissing)
	// match nodes that identify as Container Linux
	nodesToLabel := k8sutil.FilterContainerLinuxNodes(nodesMissingLabel)
	glog.V(6).Infof("Found Container Linux nodes to label: %+v", nodelist.Items)

	for _, node := range nodesToLabel {
		glog.Infof("Setting label 'agent=true' on %q", node.Name)
		if err := k8sutil.SetNodeLabels(k.nc, node.Name, enableUpdateAgentLabel); err != nil {
			glog.Errorf("Failed setting label 'agent=true' on %q", node.Name)
		}
	}
}

// updateAgent updates the agent on nodes if necessary.
//
// NOTE: the version for the agent is assumed to match the versioning scheme
// for the operator, thus our version is used to figure out the appropriate
// agent version.
// Furthermore, it's assumed that all future agent versions will be backwards
// compatible, so if the agent's version is greater than ours, it's okay.
func (k *Kontroller) runDaemonsetUpdate(agentImageRepo string) error {
	agentDaemonsets, err := k.kc.ExtensionsV1beta1().DaemonSets(k.namespace).List(v1meta.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(managedByOperatorLabels)).String(),
	})
	if err != nil {
		return err
	}

	if len(agentDaemonsets.Items) == 0 {
		// No daemonset, create it
		runErr := k.createAgentDamonset(agentImageRepo)
		if runErr != nil {
			return runErr
		}
		// runAgent succeeded, all should be well and converging now
		return nil
	}

	// There should only be one daemonset since we use a well-known name and
	// patch it each time rather than creating new ones.
	if len(agentDaemonsets.Items) > 1 {
		glog.Errorf("only expected one daemonset managed by operator; found %+v", agentDaemonsets.Items)
		return fmt.Errorf("only expected one daemonset managed by operator; found %v", len(agentDaemonsets.Items))
	}

	agentDS := agentDaemonsets.Items[0]

	var dsSemver semver.Version
	if dsVersion, ok := agentDS.Annotations[constants.AgentVersion]; ok {
		ver, err := semver.Parse(dsVersion)
		if err != nil {
			return fmt.Errorf("agent daemonset had version annotation, but it was not valid semver: %v[%v] = %v", agentDS.Name, constants.AgentVersion, dsVersion)
		}
		dsSemver = ver
	} else {
		glog.Errorf("managed daemonset did not have a version annotation: %+v", agentDS)
		return fmt.Errorf("managed daemonset did not have a version annotation")
	}

	if dsSemver.LT(version.Semver) {
		// daemonset is too old, update it
		// TODO: perform a proper rolling update rather than delete-then-recreate
		// Right now, daemonset rolling updates aren't upstream and are thus fairly
		// painful to do correctly. In addition, doing it correctly doesn't add too
		// much value unless we have corresponding detection/rollback logic.
		falseVal := false
		err := k.kc.ExtensionsV1beta1().DaemonSets(k.namespace).Delete(agentDS.Name, &v1meta.DeleteOptions{
			OrphanDependents: &falseVal, // Cascading delete
		})
		if err != nil {
			glog.Errorf("could not delete old daemonset %+v: %v", agentDS, err)
			return err
		}

		err = k.createAgentDamonset(agentImageRepo)
		if err != nil {
			glog.Errorf("could not create new daemonset: %v", err)
			return err
		}
	}

	return nil
}

func (k *Kontroller) createAgentDamonset(agentImageRepo string) error {
	_, err := k.kc.ExtensionsV1beta1().DaemonSets(k.namespace).Create(agentDaemonsetSpec(agentImageRepo))
	return err
}

func agentDaemonsetSpec(repo string) *v1beta1.DaemonSet {
	// Each agent daemonset includes the version of the agent in the selector.
	// This ensures that the 'orphan adoption' logic doesn't kick in for these
	// daemonsets.
	versionedSelector := make(map[string]string)
	for k, v := range managedByOperatorLabels {
		versionedSelector[k] = v
	}
	versionedSelector[constants.AgentVersion] = version.Version

	return &v1beta1.DaemonSet{
		ObjectMeta: v1meta.ObjectMeta{
			Name:   daemonsetName,
			Labels: managedByOperatorLabels,
			Annotations: map[string]string{
				constants.AgentVersion: version.Version,
			},
		},
		Spec: v1beta1.DaemonSetSpec{
			Selector: &v1meta.LabelSelector{MatchLabels: versionedSelector},
			Template: v1.PodTemplateSpec{
				ObjectMeta: v1meta.ObjectMeta{
					Name:   agentDefaultAppName,
					Labels: versionedSelector,
					Annotations: map[string]string{
						constants.AgentVersion: version.Version,
					},
				},
				Spec: v1.PodSpec{
					// Update the master nodes too
					Tolerations: []v1.Toleration{
						{
							Key:      "node-role.kubernetes.io/master",
							Operator: v1.TolerationOpExists,
							Effect:   v1.TaintEffectNoSchedule,
						},
					},
					Containers: []v1.Container{
						{
							Name:    "update-agent",
							Image:   agentImageName(repo),
							Command: agentCommand(),
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "var-run-dbus",
									MountPath: "/var/run/dbus",
								},
								{
									Name:      "etc-coreos",
									MountPath: "/etc/coreos",
								},
								{
									Name:      "usr-share-coreos",
									MountPath: "/usr/share/coreos",
								},
								{
									Name:      "etc-os-release",
									MountPath: "/etc/os-release",
								},
							},
							Env: []v1.EnvVar{
								{
									Name: "UPDATE_AGENT_NODE",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											FieldPath: "spec.nodeName",
										},
									},
								},
								{
									Name: "POD_NAMESPACE",
									ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "var-run-dbus",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{
									Path: "/var/run/dbus",
								},
							},
						},
						{
							Name: "etc-coreos",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{
									Path: "/etc/coreos",
								},
							},
						},
						{
							Name: "usr-share-coreos",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{
									Path: "/usr/share/coreos",
								},
							},
						},
						{
							Name: "etc-os-release",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{
									Path: "/etc/os-release",
								},
							},
						},
					},
				},
			},
		},
	}
}

func agentImageName(repo string) string {
	return fmt.Sprintf("%s:v%s", repo, version.Version)
}

func agentCommand() []string {
	return []string{"/bin/update-agent"}
}
