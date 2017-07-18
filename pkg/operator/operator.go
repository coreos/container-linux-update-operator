package operator

import (
	"fmt"
	"os"
	"time"

	"github.com/golang/glog"
	v1meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/pkg/api"
	v1api "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"

	// These should be replaced with client-go equivilents when available
	kapi "k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/clientset_generated/clientset"
	kleaderelection "k8s.io/kubernetes/pkg/client/leaderelection"
	kresourcelock "k8s.io/kubernetes/pkg/client/leaderelection/resourcelock"

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
	wantsRebootSelector = fields.ParseSelectorOrDie(constants.AnnotationRebootNeeded + "==" + constants.True + "," + constants.AnnotationRebootPaused + "!=" + constants.True)

	// stillRebootingSelector is a selector for the annotation set expected to be
	// on a node when it's in the process of rebooting
	stillRebootingSelector = fields.Set(map[string]string{
		constants.AnnotationOkToReboot:       constants.True,
		constants.AnnotationRebootNeeded:     constants.True,
		constants.AnnotationRebootInProgress: constants.True,
	}).AsSelector()
)

type Kontroller struct {
	kc *kubernetes.Clientset
	nc v1core.NodeInterface
	er record.EventRecorder

	leaderElectionClient        clientset.Interface
	leaderElectionEventRecorder record.EventRecorder
	// namespace is the kubernetes namespace any resources (e.g. locks,
	// configmaps, agents) should be created and read under.
	// It will be set to the namespace the operator is running in automatically.
	namespace string

	// Deprecated
	manageAgent    bool
	agentImageRepo string
}

// Config configures a Kontroller.
type Config struct {
	// Kubernetesc client
	Client         *kubernetes.Clientset
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
	nc := kc.Nodes()

	// create event emitter
	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: kc.Events("")})
	er := broadcaster.NewRecorder(api.Scheme, v1api.EventSource{Component: eventSourceComponent})

	leaderElectionClientConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("error creating leader election client config: %v", err)
	}
	leaderElectionClient, err := clientset.NewForConfig(leaderElectionClientConfig)
	if err != nil {
		return nil, fmt.Errorf("error creating leader election client: %v", err)
	}

	leaderElectionBroadcaster := record.NewBroadcaster()
	leaderElectionBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{
		Interface: v1core.New(leaderElectionClient.Core().RESTClient()).Events(""),
	})
	leaderElectionEventRecorder := leaderElectionBroadcaster.NewRecorder(kapi.Scheme, v1api.EventSource{
		Component: leaderElectionEventSourceComponent,
	})

	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		return nil, fmt.Errorf("unable to determine operator namespace: please ensure POD_NAMESPACE environment variable is set")
	}

	return &Kontroller{kc, nc, er, leaderElectionClient, leaderElectionEventRecorder, namespace, config.ManageAgent, config.AgentImageRepo}, nil
}

// Run starts the operator reconcilitation proces and runs until the stop
// channel is closed.
func (k *Kontroller) Run(stop <-chan struct{}) error {
	err := k.withLeaderElection()
	if err != nil {
		return err
	}

	// Before doing anytihng else, make sure the associated agent daemonset is
	// ready if it's our responsibility.
	if k.manageAgent && k.agentImageRepo != "" {
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

	// TODO(euank): this should not use an endpoint due to performance reasons.
	// See https://github.com/kubernetes/client-go/issues/28#issuecomment-284032220
	// Currently, every controller/operator uses an endpoint, so we are not alone
	// in this
	// Once https://github.com/kubernetes/kubernetes/pull/42666 is merged, update
	// to use that. There will be version-cross-compatibility-dragons.
	resLock := &kresourcelock.EndpointsLock{
		EndpointsMeta: v1meta.ObjectMeta{
			Namespace: k.namespace,
			Name:      leaderElectionResourceName,
		},
		Client: k.leaderElectionClient,
		LockConfig: kresourcelock.ResourceLockConfig{
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
		kleaderelection.RunOrDie(kleaderelection.LeaderElectionConfig{
			Lock:          resLock,
			LeaseDuration: leaderElectionLease,
			RenewDeadline: leaderElectionLease * 2 / 3,
			RetryPeriod:   leaderElectionLease / 3,
			Callbacks: kleaderelection.LeaderCallbacks{
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

	nodelist, err := k.nc.List(v1meta.ListOptions{})
	if err != nil {
		glog.Infof("Failed listing nodes %v", err)
		return
	}

	glog.V(6).Infof("Found nodes: %+v", nodelist.Items)

	justRebootedNodes := k8sutil.FilterNodesByAnnotation(nodelist.Items, justRebootedSelector)

	if len(justRebootedNodes) > 0 {
		glog.Infof("Found %d rebooted nodes, setting annotation %q to false", len(justRebootedNodes), constants.AnnotationOkToReboot)
	}

	for _, n := range justRebootedNodes {
		glog.Infof("Setting 'ok-to-reboot=false' for %q", n.Name)
		if err := k8sutil.SetNodeAnnotations(k.nc, n.Name, map[string]string{
			constants.AnnotationOkToReboot: constants.False,
		}); err != nil {
			glog.Infof("Failed setting annotation %q on node %q to false: %v", constants.AnnotationOkToReboot, n.Name, err)
		}
	}

	nodelist, err = k.nc.List(v1meta.ListOptions{})
	if err != nil {
		glog.Infof("Failed listing nodes: %v", err)
		return
	}

	// Verify no nodes are still in the process of rebooting to avoid rebooting N > maxRebootingNodes
	rebootingNodes := k8sutil.FilterNodesByAnnotation(nodelist.Items, stillRebootingSelector)
	glog.V(6).Infof("Found %+v rebooting nodes", rebootingNodes)
	if len(rebootingNodes) >= maxRebootingNodes {
		glog.Infof("Found %d (of max %d) rebooting nodes; waiting for completion", len(rebootingNodes), maxRebootingNodes)
		for _, n := range rebootingNodes {
			glog.Infof("Found node %q still rebooting, waiting", n.Name)
		}
		return
	}

	rebootableNodes := k8sutil.FilterNodesByAnnotation(nodelist.Items, wantsRebootSelector)
	if len(rebootableNodes) == 0 {
		return
	}

	remainingRebootableCount := maxRebootingNodes - len(rebootingNodes)
	// pick N of the eligible candidates to reboot
	chosenNodes := make([]*v1api.Node, 0, remainingRebootableCount)
	for i := 0; i < remainingRebootableCount && i < len(rebootableNodes); i++ {
		chosenNodes = append(chosenNodes, &rebootableNodes[i])
	}

	glog.Infof("Found %d nodes that need a reboot", len(chosenNodes))
	for _, node := range chosenNodes {
		k.markNodeRebootable(node)
	}
}

// markNodeRebootable adds ok to reboot annotations to the given node.
func (k *Kontroller) markNodeRebootable(n *v1api.Node) {
	glog.Infof("Marking %q ok to reboot", n.Name)
	if err := k8sutil.UpdateNodeRetry(k.nc, n.Name, func(node *v1api.Node) {
		if !wantsRebootSelector.Matches(fields.Set(node.Annotations)) {
			glog.Warningf("Node %v no longer wanted to reboot while we were trying to mark it so: %v", node.Name, node.Annotations)
			return
		}

		node.Annotations[constants.AnnotationOkToReboot] = constants.True
	}); err != nil {
		glog.Infof("Failed to set annotation %q on node %q: %v", constants.AnnotationOkToReboot, n.Name, err)
		return
	}
}
