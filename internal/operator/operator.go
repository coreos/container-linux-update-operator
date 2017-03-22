package operator

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/golang/glog"
	"k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	v1api "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/fields"
	"k8s.io/client-go/pkg/util/flowcontrol"
	"k8s.io/client-go/tools/record"

	// These should be replaced with client-go equivilents when available
	kapi "k8s.io/kubernetes/pkg/api"
	kinternalclientset "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	kv1core "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset/typed/core/internalversion"
	kleaderelection "k8s.io/kubernetes/pkg/client/leaderelection"
	kresourcelock "k8s.io/kubernetes/pkg/client/leaderelection/resourcelock"
	krecord "k8s.io/kubernetes/pkg/client/record"
	krest "k8s.io/kubernetes/pkg/client/restclient"

	"github.com/coreos-inc/container-linux-update-operator/internal/constants"
	"github.com/coreos-inc/container-linux-update-operator/internal/k8sutil"
)

const (
	eventReasonRebootFailed            = "RebootFailed"
	eventSourceComponent               = "update-operator"
	leaderElectionEventSourceComponent = "update-operator-leader-election"
	maxRebootingNodes                  = 1

	leaderElectionResourceName = "container-linux-update-operator-lock"

	// Arbitrarily copied from KVO
	leaderElectionLease = 90 * time.Second
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

	leaderElectionClient        kinternalclientset.Interface
	leaderElectionEventRecorder krecord.EventRecorder
}

func New() (*Kontroller, error) {
	// set up kubernetes in-cluster client
	kc, err := k8sutil.InClusterClient()
	if err != nil {
		return nil, fmt.Errorf("error creating Kubernetes client: %v", err)
	}

	// node interface
	nc := kc.Nodes()

	// create event emitter
	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: kc.Events("")})
	er := broadcaster.NewRecorder(v1api.EventSource{Component: eventSourceComponent})

	leaderElectionClientConfig, err := krest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("error creating leader election client config: %v", err)
	}
	leaderElectionClient, err := kinternalclientset.NewForConfig(leaderElectionClientConfig)
	if err != nil {
		return nil, fmt.Errorf("error creating leader election client: %v", err)
	}

	leaderElectionBroadcaster := krecord.NewBroadcaster()
	leaderElectionBroadcaster.StartRecordingToSink(&kv1core.EventSinkImpl{
		Interface: leaderElectionClient.Events(""),
	})
	leaderElectionEventRecorder := leaderElectionBroadcaster.NewRecorder(kapi.EventSource{
		Component: leaderElectionEventSourceComponent,
	})

	return &Kontroller{kc, nc, er, leaderElectionClient, leaderElectionEventRecorder}, nil
}

// withLeaderElection creates a new context which is cancelled when this
// operator does not hold a lock to operate on the cluster
func (k *Kontroller) withLeaderElection(ctx context.Context) (context.Context, error) {

	lockNamespace := os.Getenv("POD_NAMESPACE")
	if lockNamespace == "" {
		lockNamespace = "kube-system"
	}

	// TODO: a better id might be necessary.
	// Currently, KVO uses env.POD_NAME and the upstream controller-manager uses this.
	// Both end up having the same value in general, but Hostname is
	// more likely to have a value.
	id, err := os.Hostname()
	if err != nil {
		return nil, err
	}

	// TODO(euank): this should not use an endpoint due to performance reasons.
	// See https://github.com/kubernetes/client-go/issues/28#issuecomment-284032220
	// Currently, every controller/operator uses an endpoint, so we are not alone
	// in this
	// Once https://github.com/kubernetes/kubernetes/pull/42666 is merged, update
	// to use that. There will be version-cross-compatibility-dragons.
	resLock := &kresourcelock.EndpointsLock{
		EndpointsMeta: kapi.ObjectMeta{
			Namespace: lockNamespace,
			Name:      leaderElectionResourceName,
		},
		Client: k.leaderElectionClient,
		LockConfig: kresourcelock.ResourceLockConfig{
			Identity:      id,
			EventRecorder: k.leaderElectionEventRecorder,
		},
	}

	newCtx, cancelFunc := context.WithCancel(ctx)
	waitLeading := make(chan struct{})
	go func(ctx context.Context, waitLeading chan<- struct{}) {
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
					glog.V(5).Info("stopped leading; calling cancel")
					cancelFunc()
				},
			},
		})
	}(ctx, waitLeading)

	<-waitLeading
	return newCtx, nil
}

func (k *Kontroller) Run(ctx context.Context) error {
	ctx, err := k.withLeaderElection(ctx)
	if err != nil {
		return err
	}

	// Check leadership before each api write
	stillLeader := func() error {
		select {
		case <-ctx.Done():
			return fmt.Errorf("No longer leader: bailing out: %v", ctx.Err())
		default:
			return nil
		}
	}

	rl := flowcontrol.NewTokenBucketRateLimiter(0.2, 1)
	for {
		rl.Accept()
		nodelist, err := k.nc.List(v1api.ListOptions{})
		if err != nil {
			glog.Infof("Failed listing nodes %v", err)
			continue
		}

		justRebootedNodes := k8sutil.FilterNodesByAnnotation(nodelist.Items, justRebootedSelector)

		if len(justRebootedNodes) > 0 {
			glog.Infof("Found %d rebooted nodes, setting annotation %q to false", len(justRebootedNodes), constants.AnnotationOkToReboot)
		}

		for _, n := range justRebootedNodes {
			if leaderErr := stillLeader(); leaderErr != nil {
				return leaderErr
			}
			if err := k8sutil.SetNodeAnnotations(k.nc, n.Name, map[string]string{
				constants.AnnotationOkToReboot: constants.False,
			}); err != nil {
				glog.Infof("Failed setting annotation %q on node %q to false: %v", constants.AnnotationOkToReboot, n.Name, err)
			}
		}

		nodelist, err = k.nc.List(v1api.ListOptions{})
		if err != nil {
			glog.Infof("Failed listing nodes: %v", err)
			continue
		}

		// Verify no nodes are still in the process of rebooting to avoid rebooting N > maxRebootingNodes
		rebootingNodes := k8sutil.FilterNodesByAnnotation(nodelist.Items, stillRebootingSelector)
		if len(rebootingNodes) >= maxRebootingNodes {
			glog.Infof("Found %d (of max %d) rebooting nodes; waiting for completion", len(rebootingNodes), maxRebootingNodes)
			continue
		}

		rebootableNodes := k8sutil.FilterNodesByAnnotation(nodelist.Items, wantsRebootSelector)
		if len(rebootableNodes) == 0 {
			continue
		}

		remainingRebootableCount := maxRebootingNodes - len(rebootingNodes)
		// pick N of the eligible candidates to reboot
		chosenNodes := make([]*v1api.Node, 0, remainingRebootableCount)
		for i := 0; i < remainingRebootableCount && i < len(rebootableNodes); i++ {
			chosenNodes = append(chosenNodes, &rebootableNodes[i])
		}

		glog.Infof("Found %d nodes that need a reboot", len(chosenNodes))
		for _, node := range chosenNodes {
			if leaderErr := stillLeader(); leaderErr != nil {
				return leaderErr
			}
			k.markNodeRebootable(node)
		}

	}
}

func (k *Kontroller) markNodeRebootable(n *v1api.Node) {
	glog.V(4).Infof("marking %s ok to reboot", n.Name)
	if err := k8sutil.UpdateNodeRetry(k.nc, n.Name, func(node *v1api.Node) {
		// TODO; reuse selector if I can figure out how to apply it to a single node
		if node.Annotations[constants.AnnotationOkToReboot] == constants.True {
			glog.Warningf("Node %v became rebootable while we were trying to mark it so", node.Name)
			return
		}
		if node.Annotations[constants.AnnotationRebootNeeded] != constants.True {
			glog.Warningf("Node %v became not-ok-for-reboot while trying to mark it ready", node.Name)
			return
		}
		node.Annotations[constants.AnnotationOkToReboot] = constants.True
	}); err != nil {
		glog.Infof("Failed to set annotation %q on node %q: %v", constants.AnnotationOkToReboot, n.Name, err)
		return
	}
}
