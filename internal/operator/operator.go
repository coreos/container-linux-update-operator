package operator

import (
	"fmt"
	"log"
	"time"

	"k8s.io/client-go/1.5/kubernetes"
	v1core "k8s.io/client-go/1.5/kubernetes/typed/core/v1"
	"k8s.io/client-go/1.5/pkg/api"
	v1api "k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/fields"
	"k8s.io/client-go/1.5/pkg/labels"
	"k8s.io/client-go/1.5/pkg/util/flowcontrol"
	"k8s.io/client-go/1.5/pkg/watch"
	"k8s.io/client-go/1.5/tools/record"

	"github.com/coreos-inc/container-linux-update-operator/internal/constants"
	"github.com/coreos-inc/container-linux-update-operator/internal/k8sutil"
)

const (
	eventReasonRebootFailed = "RebootFailed"
	eventSourceComponent    = "update-operator"
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

	// make tpr
	if err := k8sutil.CreateTPR(kc, "reboot-group.coreos.com", "v1", "Group of CoreOS nodes"); err != nil {
		return nil, fmt.Errorf("error creating TPR: %v", err)
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

		// find nodes which rebooted, reset constants.LabelOkToReboot
		ls := labels.Set(map[string]string{
			constants.LabelOkToReboot:       "true",
			constants.LabelRebootNeeded:     "false",
			constants.LabelRebootInProgress: "false",
		})

		nodes, err := k.nc.List(api.ListOptions{LabelSelector: ls.AsSelector()})
		if err != nil {
			log.Printf("Failed listing nodes with labels %q: %v", ls, err)
			continue
		}

		if len(nodes.Items) > 0 {
			log.Printf("Found %d rebooted nodes, setting label %q to false", len(nodes.Items), constants.LabelOkToReboot)
		}

		for _, n := range nodes.Items {
			if err := k8sutil.SetNodeLabels(k.nc, n.Name, map[string]string{
				constants.LabelOkToReboot: "false",
			}); err != nil {
				log.Printf("Failed setting label %q on node %q to false: %v", constants.LabelOkToReboot, n.Name, err)
			}
		}

		// find N nodes that want to reboot
		ls = labels.Set(map[string]string{
			constants.LabelRebootNeeded: "true",
		})

		nodes, err = k.nc.List(api.ListOptions{LabelSelector: ls.AsSelector()})
		if err != nil {
			log.Printf("Failed listing nodes with label %q: %v", constants.LabelRebootNeeded, err)
			continue
		}

		// pick N of these machines
		// TODO: for now, synchronous with N == 1. might be async w/ a channel in the future to handle N > 1
		if len(nodes.Items) == 0 {
			continue
		}

		n := nodes.Items[0]

		log.Printf("Found %d nodes that need a reboot, rebooting %q", len(nodes.Items), n.Name)

		k.handleReboot(&n)
	}
}

func (k *Kontroller) handleReboot(n *v1api.Node) {
	// node wants to reboot, so let it.
	if err := k8sutil.SetNodeLabels(k.nc, n.Name, map[string]string{
		constants.LabelOkToReboot: "true",
	}); err != nil {
		log.Printf("Failed to set label %q on node %q: %v", constants.LabelOkToReboot, n.Name, err)
		return
	}

	// wait for it to come back...
	watcher, err := k.nc.Watch(api.ListOptions{
		FieldSelector:   fields.OneTermEqualSelector("metadata.name", n.Name),
		ResourceVersion: n.ResourceVersion,
	})

	conds := []watch.ConditionFunc{
		k8sutil.NodeLabelCondition(constants.LabelOkToReboot, "true"),
		k8sutil.NodeLabelCondition(constants.LabelRebootNeeded, "false"),
		k8sutil.NodeLabelCondition(constants.LabelRebootInProgress, "false"),
	}
	_, err = watch.Until(time.Hour*1, watcher, conds...)
	if err != nil {
		log.Printf("Waiting for label %q on node %q failed: %v", constants.LabelOkToReboot, n.Name, err)
		log.Printf("Failed to wait for successful reboot of node %q", n.Name)

		k.er.Eventf(n, api.EventTypeWarning, eventReasonRebootFailed, "Timed out waiting for node to return after a reboot")
	}

	// node rebooted successfully, or at least set the labels we expected from klocksmith after a reboot.
}
