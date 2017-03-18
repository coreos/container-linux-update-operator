package operator

import (
	"fmt"

	"github.com/golang/glog"
	"k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	v1api "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/fields"
	"k8s.io/client-go/pkg/util/flowcontrol"
	"k8s.io/client-go/tools/record"

	"github.com/coreos-inc/container-linux-update-operator/internal/constants"
	"github.com/coreos-inc/container-linux-update-operator/internal/k8sutil"
)

const (
	eventReasonRebootFailed = "RebootFailed"
	eventSourceComponent    = "update-operator"
	maxRebootingNodes       = 1
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

	return &Kontroller{kc, nc, er}, nil
}

func (k *Kontroller) Run() error {
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
