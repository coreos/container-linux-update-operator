package klocksmith

import (
	"fmt"
	"log"
	"time"

	"k8s.io/client-go/1.4/kubernetes"
	v1core "k8s.io/client-go/1.4/kubernetes/typed/core/v1"
	"k8s.io/client-go/1.4/pkg/api"
	v1api "k8s.io/client-go/1.4/pkg/api/v1"
	"k8s.io/client-go/1.4/pkg/fields"
	"k8s.io/client-go/1.4/pkg/labels"
	"k8s.io/client-go/1.4/pkg/util/flowcontrol"
	"k8s.io/client-go/1.4/pkg/watch"

	"github.com/coreos-inc/klocksmith/internal/k8sutil"
)

type Kontroller struct {
	kc *kubernetes.Clientset
	nc v1core.NodeInterface
}

func NewKontroller() (*Kontroller, error) {
	// set up kubernetes in-cluster client
	kc, err := k8sutil.InClusterClient()
	if err != nil {
		return nil, fmt.Errorf("error creating kubernetes client: %v", err)
	}

	// node interface
	nc := kc.Nodes()

	return &Kontroller{kc, nc}, nil
}

func (k *Kontroller) Run() error {
	rl := flowcontrol.NewTokenBucketRateLimiter(0.2, 1)
	for {
		rl.Accept()

		// find nodes which rebooted, reset labelOkToReboot
		ls := labels.Set(map[string]string{
			labelOkToReboot:       "true",
			labelRebootNeeded:     "false",
			labelRebootInProgress: "false",
		})

		nodes, err := k.nc.List(api.ListOptions{LabelSelector: ls.AsSelector()})
		if err != nil {
			log.Printf("Failed listing nodes with labels %q: %v", ls, err)
			continue
		}

		if len(nodes.Items) > 0 {
			log.Printf("Found %d rebooted nodes, setting label %q to false", len(nodes.Items), labelOkToReboot)
		}

		for _, n := range nodes.Items {
			if err := k8sutil.SetNodeLabels(k.nc, n.Name, map[string]string{
				labelOkToReboot: "false",
			}); err != nil {
				log.Printf("Failed setting label %q on node %q to false: %v", labelOkToReboot, n.Name, err)
			}
		}

		// find N nodes that want to reboot
		ls = labels.Set(map[string]string{
			labelRebootNeeded: "true",
		})

		nodes, err = k.nc.List(api.ListOptions{LabelSelector: ls.AsSelector()})
		if err != nil {
			log.Printf("Failed listing nodes with label %q: %v", labelRebootNeeded, err)
			continue
		}

		// pick N of these machines
		// TODO: for now, synchronous with N == 1. might be async w/ a channel in the future to handle N > 1
		if len(nodes.Items) == 0 {
			continue
		}

		n := nodes.Items[0]

		log.Printf("Found %d nodes that need a reboot, rebooting %q", len(nodes.Items), n.Name)

		k.handleReboot(n)
	}
}

func (k *Kontroller) handleReboot(n v1api.Node) {
	// node wants to reboot, so let it.
	if err := k8sutil.SetNodeLabels(k.nc, n.Name, map[string]string{
		labelOkToReboot: "true",
	}); err != nil {
		log.Printf("Failed to set label %q on node %q: %v", labelOkToReboot, n.Name, err)
		return
	}

	// wait for it to come back...
	watcher, err := k.nc.Watch(api.ListOptions{
		FieldSelector:   fields.OneTermEqualSelector("metadata.name", n.Name),
		ResourceVersion: n.ResourceVersion,
	})

	// hopefully 1 hours is enough time between indicating the
	// node can reboot and it successfully rebooting
	conds := []watch.ConditionFunc{
		k8sutil.NodeLabelCondition(labelOkToReboot, "true"),
		k8sutil.NodeLabelCondition(labelRebootNeeded, "false"),
		k8sutil.NodeLabelCondition(labelRebootInProgress, "false"),
	}
	_, err = watch.Until(time.Hour*1, watcher, conds...)
	if err != nil {
		log.Printf("Waiting for label %q on node %q failed: %v", labelOkToReboot, n.Name, err)
		log.Printf("Failed to wait for successful reboot of node %q", n.Name)
	}

	// node rebooted successfully, or at least set the labels we expected from klocksmith after a reboot.
}
