package operator

import (
	"fmt"
	"os"
	"time"

	"github.com/golang/glog"
	v1api "k8s.io/api/core/v1"
	v1meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"

	"github.com/coreos/container-linux-update-operator/pkg/constants"
	"github.com/coreos/container-linux-update-operator/pkg/k8sutil"
)

const (
	eventReasonRebootFailed            = "RebootFailed"
	eventSourceComponent               = "update-operator"
	leaderElectionEventSourceComponent = "update-operator-leader-election"
	// agentDefaultAppName is the label value for the 'app' key that agents are
	// expected to be labeled with.
	agentDefaultAppName = "container-linux-update-agent"
	maxRebootingNodes   = 1

	leaderElectionResourceName = "container-linux-update-operator-lock"

	// Arbitrarily copied from KVO
	leaderElectionLease = 90 * time.Second
	// ReconciliationPeriod
	reconciliationPeriod = 30 * time.Second
)

var (
	// justRebootedSelector is a selector for combination of annotations
	// expected to be on a node after it has completed a reboot.
	//
	// The update-operator sets constants.AnnotationOkToReboot to true to
	// trigger a reboot, and the update-agent sets
	// constants.AnnotationRebootNeeded and
	// constants.AnnotationRebootInProgress to false when it has finished.
	justRebootedSelector = fields.Set(map[string]string{
		constants.AnnotationOkToReboot:       constants.True,
		constants.AnnotationRebootNeeded:     constants.False,
		constants.AnnotationRebootInProgress: constants.False,
	}).AsSelector()

	// wantsRebootSelector is a selector for the annotation expected to be on a node when it wants to be rebooted.
	//
	// The update-agent sets constants.AnnotationRebootNeeded to true when
	// it would like to reboot, and false when it starts up.
	//
	// If constants.AnnotationRebootPaused is set to "true", the update-agent will not consider it for rebooting.
	wantsRebootSelector = fields.ParseSelectorOrDie(constants.AnnotationRebootNeeded + "==" + constants.True +
		"," + constants.AnnotationRebootPaused + "!=" + constants.True +
		"," + constants.AnnotationOkToReboot + "!=" + constants.True +
		"," + constants.AnnotationRebootInProgress + "!=" + constants.True)

	// stillRebootingSelector is a selector for the annotation set expected to be
	// on a node when it's in the process of rebooting
	stillRebootingSelector = fields.Set(map[string]string{
		constants.AnnotationOkToReboot:   constants.True,
		constants.AnnotationRebootNeeded: constants.True,
	}).AsSelector()

	// beforeRebootReq requires a node to be waiting for before reboot checks to complete
	beforeRebootReq = k8sutil.NewRequirementOrDie(constants.LabelBeforeReboot, selection.In, []string{constants.True})

	// afterRebootReq requires a node to be waiting for after reboot checks to complete
	afterRebootReq = k8sutil.NewRequirementOrDie(constants.LabelAfterReboot, selection.In, []string{constants.True})

	// notBeforeRebootReq and notAfterRebootReq are the inverse of the above checks
	notBeforeRebootReq = k8sutil.NewRequirementOrDie(constants.LabelBeforeReboot, selection.NotIn, []string{constants.True})
	notAfterRebootReq  = k8sutil.NewRequirementOrDie(constants.LabelAfterReboot, selection.NotIn, []string{constants.True})
)

type Kontroller struct {
	kc kubernetes.Interface
	nc v1core.NodeInterface
	er record.EventRecorder

	// annotations to look for before and after reboots
	beforeRebootAnnotations []string
	afterRebootAnnotations  []string

	leaderElectionClient        *kubernetes.Clientset
	leaderElectionEventRecorder record.EventRecorder
	// namespace is the kubernetes namespace any resources (e.g. locks,
	// configmaps, agents) should be created and read under.
	// It will be set to the namespace the operator is running in automatically.
	namespace string

	// auto-label Container Linux nodes for migration compatability
	autoLabelContainerLinux bool

	// Deprecated
	manageAgent    bool
	agentImageRepo string
}

// Config configures a Kontroller.
type Config struct {
	// Kubernetesc client
	Client kubernetes.Interface
	// migration compatability
	AutoLabelContainerLinux bool
	// annotations to look for before and after reboots
	BeforeRebootAnnotations []string
	AfterRebootAnnotations  []string
	// Deprecated
	ManageAgent    bool
	AgentImageRepo string
}

// New initializes a new Kontroller.
func New(config Config) (*Kontroller, error) {
	// kubernetes client
	if config.Client == nil {
		return nil, fmt.Errorf("Kubernetes client must not be nil")
	}
	kc := config.Client

	// node interface
	nc := kc.CoreV1().Nodes()

	// create event emitter
	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: kc.CoreV1().Events("")})
	er := broadcaster.NewRecorder(runtime.NewScheme(), v1api.EventSource{Component: eventSourceComponent})

	leaderElectionClientConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("error creating leader election client config: %v", err)
	}
	leaderElectionClient, err := kubernetes.NewForConfig(leaderElectionClientConfig)
	if err != nil {
		return nil, fmt.Errorf("error creating leader election client: %v", err)
	}

	leaderElectionBroadcaster := record.NewBroadcaster()
	leaderElectionBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{
		Interface: v1core.New(leaderElectionClient.Core().RESTClient()).Events(""),
	})
	leaderElectionEventRecorder := leaderElectionBroadcaster.NewRecorder(runtime.NewScheme(), v1api.EventSource{
		Component: leaderElectionEventSourceComponent,
	})

	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		return nil, fmt.Errorf("unable to determine operator namespace: please ensure POD_NAMESPACE environment variable is set")
	}

	return &Kontroller{
		kc: kc,
		nc: nc,
		er: er,
		beforeRebootAnnotations:     config.BeforeRebootAnnotations,
		afterRebootAnnotations:      config.AfterRebootAnnotations,
		leaderElectionClient:        leaderElectionClient,
		leaderElectionEventRecorder: leaderElectionEventRecorder,
		namespace:                   namespace,
		autoLabelContainerLinux:     config.AutoLabelContainerLinux,
		manageAgent:                 config.ManageAgent,
		agentImageRepo:              config.AgentImageRepo,
	}, nil
}

// Run starts the operator reconcilitation proces and runs until the stop
// channel is closed.
func (k *Kontroller) Run(stop <-chan struct{}) error {
	err := k.withLeaderElection()
	if err != nil {
		return err
	}

	// start Container Linux node auto-labeler
	if k.autoLabelContainerLinux {
		go wait.Until(k.legacyLabeler, reconciliationPeriod, stop)
	}

	// Before doing anytihng else, make sure the associated agent daemonset is
	// ready if it's our responsibility.
	if k.manageAgent && k.agentImageRepo != "" {
		// create or update the update-agent daemonset
		err := k.runDaemonsetUpdate(k.agentImageRepo)
		if err != nil {
			glog.Errorf("unable to ensure managed agents are ready: %v", err)
			return err
		}
	}

	glog.V(5).Info("starting controller")

	// call the process loop each period, until stop is closed
	wait.Until(k.process, reconciliationPeriod, stop)

	glog.V(5).Info("stopping controller")
	return nil
}

// withLeaderElection creates a new context which is cancelled when this
// operator does not hold a lock to operate on the cluster
func (k *Kontroller) withLeaderElection() error {
	// TODO: a better id might be necessary.
	// Currently, KVO uses env.POD_NAME and the upstream controller-manager uses this.
	// Both end up having the same value in general, but Hostname is
	// more likely to have a value.
	id, err := os.Hostname()
	if err != nil {
		return err
	}

	resLock := &resourcelock.ConfigMapLock{
		ConfigMapMeta: v1meta.ObjectMeta{
			Namespace: k.namespace,
			Name:      leaderElectionResourceName,
		},
		Client: k.leaderElectionClient,
		LockConfig: resourcelock.ResourceLockConfig{
			Identity:      id,
			EventRecorder: k.leaderElectionEventRecorder,
		},
	}

	waitLeading := make(chan struct{})
	go func(waitLeading chan<- struct{}) {
		// Lease values inspired by a combination of
		// https://github.com/kubernetes/kubernetes/blob/f7c07a121d2afadde7aa15b12a9d02858b30a0a9/pkg/apis/componentconfig/v1alpha1/defaults.go#L163-L174
		// and the KVO values
		// See also
		// https://github.com/kubernetes/kubernetes/blob/fc31dae165f406026142f0dd9a98cada8474682a/pkg/client/leaderelection/leaderelection.go#L17
		leaderelection.RunOrDie(leaderelection.LeaderElectionConfig{
			Lock:          resLock,
			LeaseDuration: leaderElectionLease,
			RenewDeadline: leaderElectionLease * 2 / 3,
			RetryPeriod:   leaderElectionLease / 3,
			Callbacks: leaderelection.LeaderCallbacks{
				OnStartedLeading: func(stop <-chan struct{}) {
					glog.V(5).Info("started leading")
					waitLeading <- struct{}{}
				},
				OnStoppedLeading: func() {
					glog.Fatalf("leaderelection lost")
				},
			},
		})
	}(waitLeading)

	<-waitLeading
	return nil
}

// process performs the reconcilitation to coordinate reboots.
func (k *Kontroller) process() {
	glog.V(4).Info("Going through a loop cycle")

	// first make sure that all of our nodes are in a well-defined state with
	// respect to our annotations and labels, and if they are not, then try to
	// fix them.
	glog.V(4).Info("Cleaning up node state")
	err := k.cleanupState()
	if err != nil {
		glog.Errorf("Failed to cleanup node state: %v", err)
		return
	}

	// find nodes with the after-reboot=true label and check if all provided
	// annotations are set. if all annotations are set to true then remove the
	// after-reboot=true label and set reboot-ok=false, telling the agent that
	// the reboot has completed.
	glog.V(4).Info("Checking if configured after-reboot annotations are set to true")
	err = k.checkAfterReboot()
	if err != nil {
		glog.Errorf("Failed to check after reboot: %v", err)
		return
	}

	// find nodes which just rebooted but haven't run after-reboot checks.
	// remove after-reboot annotations and add the after-reboot=true label.
	glog.V(4).Info("Labeling rebooted nodes with after-reboot label")
	err = k.markAfterReboot()
	if err != nil {
		glog.Errorf("Failed to update recently rebooted nodes: %v", err)
		return
	}

	// find nodes with the before-reboot=true label and check if all provided
	// annotations are set. if all annotations are set to true then remove the
	// before-reboot=true label and set reboot=ok=true, telling the agent it's
	// time to reboot.
	glog.V(4).Info("Checking if configured before-reboot annotations are set to true")
	err = k.checkBeforeReboot()
	if err != nil {
		glog.Errorf("Failed to check before reboot: %v", err)
		return
	}

	// take some number of the rebootable nodes. remove before-reboot
	// annotations and add the before-reboot=true label.
	glog.V(4).Info("Labeling rebootable nodes with before-reboot label")
	err = k.markBeforeReboot()
	if err != nil {
		glog.Errorf("Failed to update rebootable nodes: %v", err)
		return
	}
}

// cleanupState attempts to make sure nodes are in a well-defined state before
// performing state changes on them.
// If there is an error getting the list of nodes or updating any of them, an
// error is immediately returned.
func (k *Kontroller) cleanupState() error {
	nodelist, err := k.nc.List(v1meta.ListOptions{})
	if err != nil {
		return fmt.Errorf("Failed listing nodes: %v", err)
	}

	for _, n := range nodelist.Items {
		err = k8sutil.UpdateNodeRetry(k.nc, n.Name, func(node *v1api.Node) {
			// make sure that nodes with the before-reboot label actually
			// still wants to reboot
			if _, exists := node.Labels[constants.LabelBeforeReboot]; exists {
				if !wantsRebootSelector.Matches(fields.Set(node.Annotations)) {
					glog.Warningf("Node %v no longer wanted to reboot while we were trying to label it so: %v", node.Name, node.Annotations)
					delete(node.Labels, constants.LabelBeforeReboot)
					for _, annotation := range k.beforeRebootAnnotations {
						delete(node.Annotations, annotation)
					}
				}
			}
		})
		if err != nil {
			return fmt.Errorf("Failed to cleanup node %q: %v", n.Name, err)
		}
	}

	return nil
}

// checkBeforeReboot gets all nodes with the before-reboot=true label and checks
// if all of the configured before-reboot annotations are set to true. If they
// are, it deletes the before-reboot=true label and sets reboot-ok=true to tell
// the agent that it is ready to start the actual reboot process.
// If it goes to set reboot-ok=true and finds that the node no longer wants a
// reboot, then it just deletes the before-reboot=true label.
// If there is an error getting the list of nodes or updating any of them, an
// error is immediately returned.
func (k *Kontroller) checkBeforeReboot() error {
	nodelist, err := k.nc.List(v1meta.ListOptions{})
	if err != nil {
		return fmt.Errorf("Failed listing nodes: %v", err)
	}

	preRebootNodes := k8sutil.FilterNodesByRequirement(nodelist.Items, beforeRebootReq)

	for _, n := range preRebootNodes {
		if hasAllAnnotations(n, k.beforeRebootAnnotations) {
			glog.V(4).Infof("Deleting label %q for %q", constants.LabelBeforeReboot, n.Name)
			glog.V(4).Infof("Setting annotation %q to true for %q", constants.AnnotationOkToReboot, n.Name)
			err = k8sutil.UpdateNodeRetry(k.nc, n.Name, func(node *v1api.Node) {
				delete(node.Labels, constants.LabelBeforeReboot)
				// cleanup the before-reboot annotations
				for _, annotation := range k.beforeRebootAnnotations {
					glog.V(4).Info("Deleting annotation %q from node %q", annotation, node.Name)
					delete(node.Annotations, annotation)
				}
				node.Annotations[constants.AnnotationOkToReboot] = constants.True
			})
			if err != nil {
				return fmt.Errorf("Failed to update node %q: %v", n.Name, err)
			}
		}
	}

	return nil
}

// checkAfterReboot gets all nodes with the after-reboot=true label and checks
// if  all of the configured after-reboot annotations are set to true. If they
// are, it deletes the after-reboot=true label and sets reboot-ok=false to tell
// the agent that it has completed it's reboot successfully.
// If there is an error getting the list of nodes or updating any of them, an
// error is immediately returned.
func (k *Kontroller) checkAfterReboot() error {
	nodelist, err := k.nc.List(v1meta.ListOptions{})
	if err != nil {
		return fmt.Errorf("Failed listing nodes: %v", err)
	}

	postRebootNodes := k8sutil.FilterNodesByRequirement(nodelist.Items, afterRebootReq)

	for _, n := range postRebootNodes {
		if hasAllAnnotations(n, k.afterRebootAnnotations) {
			glog.V(4).Infof("Deleting label %q for %q", constants.LabelAfterReboot, n.Name)
			glog.V(4).Infof("Setting annotation %q to false for %q", constants.AnnotationOkToReboot, n.Name)
			err = k8sutil.UpdateNodeRetry(k.nc, n.Name, func(node *v1api.Node) {
				delete(node.Labels, constants.LabelAfterReboot)
				// cleanup the after-reboot annotations
				for _, annotation := range k.afterRebootAnnotations {
					glog.V(4).Info("Deleting annotation %q from node %q", annotation, node.Name)
					delete(node.Annotations, annotation)
				}
				node.Annotations[constants.AnnotationOkToReboot] = constants.False
			})
			if err != nil {
				return fmt.Errorf("Failed to update node %q: %v", n.Name, err)
			}
		}
	}

	return nil
}

// markBeforeReboot gets nodes which want to reboot and marks them with the
// before-reboot=true label. This is considered the beginning of the reboot
// process from the perspective of the update-operator. It will only mark
// nodes with this label up to the maximum number of concurrently rebootable
// nodes as configured with the maxRebootingNodes constant.
// It cleans up the before-reboot annotations before it applies the label, in
// case there are any left over from the last reboot.
// If there is an error getting the list of nodes or updating any of them, an
// error is immediately returned.
func (k *Kontroller) markBeforeReboot() error {
	nodelist, err := k.nc.List(v1meta.ListOptions{})
	if err != nil {
		return fmt.Errorf("Failed listing nodes: %v", err)
	}

	// find nodes which are still rebooting
	rebootingNodes := k8sutil.FilterNodesByAnnotation(nodelist.Items, stillRebootingSelector)
	// nodes running before and after reboot checks are still considered to be "rebooting" to us
	beforeRebootNodes := k8sutil.FilterNodesByRequirement(nodelist.Items, beforeRebootReq)
	rebootingNodes = append(rebootingNodes, beforeRebootNodes...)
	afterRebootNodes := k8sutil.FilterNodesByRequirement(nodelist.Items, afterRebootReq)
	rebootingNodes = append(rebootingNodes, afterRebootNodes...)

	// Verify the number of currently rebooting nodes is less than the the maximum number
	if len(rebootingNodes) >= maxRebootingNodes {
		for _, n := range rebootingNodes {
			glog.Infof("Found node %q still rebooting, waiting", n.Name)
		}
		glog.Infof("Found %d (of max %d) rebooting nodes; waiting for completion", len(rebootingNodes), maxRebootingNodes)
		return nil
	}

	// find nodes which want to reboot
	rebootableNodes := k8sutil.FilterNodesByAnnotation(nodelist.Items, wantsRebootSelector)
	rebootableNodes = k8sutil.FilterNodesByRequirement(rebootableNodes, notBeforeRebootReq)

	// Don't even bother if rebootableNodes is empty. We wouldn't do anything anyway.
	if len(rebootableNodes) == 0 {
		return nil
	}

	// find the number of nodes we can tell to reboot
	remainingRebootableCount := maxRebootingNodes - len(rebootingNodes)

	// choose some number of nodes
	chosenNodes := make([]*v1api.Node, 0, remainingRebootableCount)
	for i := 0; i < remainingRebootableCount && i < len(rebootableNodes); i++ {
		chosenNodes = append(chosenNodes, &rebootableNodes[i])
	}

	// set before-reboot=true for the chosen nodes
	glog.Infof("Found %d nodes that need a reboot", len(chosenNodes))
	for _, n := range chosenNodes {
		err = k.mark(n.Name, constants.LabelBeforeReboot, k.beforeRebootAnnotations)
		if err != nil {
			return fmt.Errorf("Failed to label node for before reboot checks: %v", err)
		}
		if len(k.beforeRebootAnnotations) > 0 {
			glog.Infof("Waiting for before-reboot annotations on node %q: %v", n.Name, k.beforeRebootAnnotations)
		}
	}

	return nil
}

// markAfterReboot gets nodes which have completed rebooting and marks them with
// the after-reboot=true label. A node with the after-reboot=true label is still
// considered to be rebooting from the perspective of the update-operator, even
// though it has completed rebooting from the machines perspective.
// It cleans up the after-reboot annotations before it applies the label, in
// case there are any left over from the last reboot.
// If there is an error getting the list of nodes or updating any of them, an
// error is immediately returned.
func (k *Kontroller) markAfterReboot() error {
	nodelist, err := k.nc.List(v1meta.ListOptions{})
	if err != nil {
		return fmt.Errorf("Failed listing nodes: %v", err)
	}

	// find nodes which just rebooted
	justRebootedNodes := k8sutil.FilterNodesByAnnotation(nodelist.Items, justRebootedSelector)
	// also filter out any nodes that are already labeled with after-reboot=true
	justRebootedNodes = k8sutil.FilterNodesByRequirement(justRebootedNodes, notAfterRebootReq)

	glog.Infof("Found %d rebooted nodes", len(justRebootedNodes))

	// for all the nodes which just rebooted, remove any old annotations and add the after-reboot=true label
	for _, n := range justRebootedNodes {
		err = k.mark(n.Name, constants.LabelAfterReboot, k.afterRebootAnnotations)
		if err != nil {
			return fmt.Errorf("Failed to label node for after reboot checks: %v", err)
		}
		if len(k.afterRebootAnnotations) > 0 {
			glog.Infof("Waiting for after-reboot annotations on node %q: %v", n.Name, k.afterRebootAnnotations)
		}
	}

	return nil
}

func (k *Kontroller) mark(nodeName string, label string, annotations []string) error {
	glog.V(4).Infof("Deleting annotations %v for %q", annotations, nodeName)
	glog.V(4).Infof("Setting label %q to %q for node %q", label, constants.True, nodeName)
	err := k8sutil.UpdateNodeRetry(k.nc, nodeName, func(node *v1api.Node) {
		for _, annotation := range annotations {
			delete(node.Annotations, annotation)
		}
		node.Labels[label] = constants.True
	})
	if err != nil {
		return fmt.Errorf("Failed to set %q to %q on node %q: %v", label, constants.True, nodeName, err)
	}

	return nil
}

func hasAllAnnotations(node v1api.Node, annotations []string) bool {
	nodeAnnotations := node.GetAnnotations()
	for _, annotation := range annotations {
		value, ok := nodeAnnotations[annotation]
		if !ok || value != constants.True {
			return false
		}
	}
	return true
}
