package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/coreos/go-systemd/login1"
	"github.com/golang/glog"
	v1meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/pkg/api/v1"

	"github.com/coreos/container-linux-update-operator/pkg/constants"
	"github.com/coreos/container-linux-update-operator/pkg/drain"
	"github.com/coreos/container-linux-update-operator/pkg/hook"
	"github.com/coreos/container-linux-update-operator/pkg/k8sutil"
	"github.com/coreos/container-linux-update-operator/pkg/updateengine"
)

type Klocksmith struct {
	node string
	kc   *kubernetes.Clientset
	nc   v1core.NodeInterface
	ue   *updateengine.Client
	lc   *login1.Conn
	hook *hook.Hook
}

var (
	shouldRebootSelector = fields.Set(map[string]string{
		constants.AnnotationOkToReboot:   constants.True,
		constants.AnnotationRebootNeeded: constants.True,
	}).AsSelector()
)

func New(node string) (*Klocksmith, error) {
	// set up kubernetes in-cluster client
	kc, err := k8sutil.InClusterClient()
	if err != nil {
		return nil, fmt.Errorf("error creating kubernetes client: %v", err)
	}

	// node interface
	nc := kc.Nodes()

	// set up update_engine client
	ue, err := updateengine.New()
	if err != nil {
		return nil, fmt.Errorf("error establishing connection to update_engine dbus: %v", err)
	}

	// set up login1 client for our eventual reboot
	lc, err := login1.New()
	if err != nil {
		return nil, fmt.Errorf("error establishing connection to logind dbus: %v", err)
	}

	h, err := hook.New("cluo-pre-reboot-hook.target") // TODO(sdemos): this should be configurable
	if err != nil {
		return nil, fmt.Errorf("error establishing connection to systemd dbus: %v", err)
	}

	return &Klocksmith{node, kc, nc, ue, lc, h}, nil
}

// updateStatusCallback receives Status messages from update engine. If the
// status is UpdateStatusUpdatedNeedReboot, indicate that with a label on our
// node.
func (k *Klocksmith) updateStatusCallback(s updateengine.Status) {
	glog.Info("Updating status")
	// update our status
	anno := map[string]string{
		constants.AnnotationStatus:          s.CurrentOperation,
		constants.AnnotationLastCheckedTime: fmt.Sprintf("%d", s.LastCheckedTime),
		constants.AnnotationNewVersion:      s.NewVersion,
	}

	// indicate we need a reboot
	if s.CurrentOperation == updateengine.UpdateStatusUpdatedNeedReboot {
		glog.Info("Indicating a reboot is needed")
		anno[constants.AnnotationRebootNeeded] = constants.True
	}

	wait.PollUntil(10*time.Second, func() (bool, error) {
		if err := k8sutil.SetNodeAnnotations(k.nc, k.node, anno); err != nil {
			glog.Errorf("Failed to set annotation %q: %v", constants.AnnotationStatus, err)
			return false, nil
		}

		return true, nil
	}, wait.NeverStop)
}

// setInfoLabels labels our node with helpful info about Container Linux.
func (k *Klocksmith) setInfoLabels() error {
	vi, err := k8sutil.GetVersionInfo()
	if err != nil {
		return fmt.Errorf("failed to get version info: %v", err)
	}

	labels := map[string]string{
		constants.LabelID:      vi.ID,
		constants.LabelGroup:   vi.Group,
		constants.LabelVersion: vi.Version,
	}

	if err := k8sutil.SetNodeLabels(k.nc, k.node, labels); err != nil {
		return err
	}

	return nil
}

// Run runs klocksmithd, reacting to the update_engine reboot signal by
// draining pods on this kubernetes node and rebooting.
//
// TODO(mischief): try to be more resilient against transient failures
func (k *Klocksmith) Run() error {
	glog.Info("Setting info labels")
	if err := k.setInfoLabels(); err != nil {
		return fmt.Errorf("failed to set node info: %v", err)
	}

	// we are schedulable now.
	glog.Info("Marking node as schedulable")
	if err := k8sutil.Unschedulable(k.nc, k.node, false); err != nil {
		return err
	}

	// set coreos.com/update1/reboot-in-progress=false and
	// coreos.com/update1/reboot-needed=false
	anno := map[string]string{
		constants.AnnotationRebootInProgress: constants.False,
		constants.AnnotationRebootNeeded:     constants.False,
	}
	glog.Infof("Setting annotations %#v", anno)
	if err := k8sutil.SetNodeAnnotations(k.nc, k.node, anno); err != nil {
		return err
	}
	// Since we set 'reboot-needed=false', 'ok-to-reboot' should clear.
	// Wait for it to do so, else we might start reboot-looping
	if err := k.waitForNotOkToReboot(); err != nil {
		return err
	}

	// watch update engine for status updates
	go k.watchUpdateStatus(k.updateStatusCallback)

	// block until constants.AnnotationOkToReboot is set
	for {
		glog.Infof("Waiting for ok-to-reboot from controller...")
		err := k.waitForOkToReboot()
		if err == nil {
			// time to reboot
			break
		}
		glog.Warningf("error waiting for an ok-to-reboot: %v", err)
	}

	// call the pre-reboot hook target
	glog.Info("Calling pre-reboot target")
	if err := k.hook.CompleteUnit(); err != nil {
		return err
	}

	// set constants.AnnotationRebootInProgress and drain self
	anno = map[string]string{
		constants.AnnotationRebootInProgress: constants.True,
	}

	glog.Infof("Setting annotations %#v", anno)
	if err := k8sutil.SetNodeAnnotations(k.nc, k.node, anno); err != nil {
		return err
	}

	// drain self equates to:
	// 1. set Unschedulable
	// 2. delete all pods
	// unlike `kubectl drain`, we do not care about emptyDir or orphan pods
	// ('any pods that are neither mirror pods nor managed by
	// ReplicationController, ReplicaSet, DaemonSet or Job')

	glog.Info("Marking node as unschedulable")
	if err := k8sutil.Unschedulable(k.nc, k.node, true); err != nil {
		return err
	}

	glog.Info("Getting pod list for deletion")
	pods, err := k.getPodsForDeletion()
	if err != nil {
		return err
	}

	// delete the pods.
	// TODO(mischief): explicitly don't terminate self? we'll probably just be a
	// mirror pod or daemonset anyway..
	glog.Infof("Deleting %d pods", len(pods))
	deleteOptions := v1meta.NewDeleteOptions(30)
	for _, pod := range pods {
		glog.Infof("Terminating pod %q...", pod.Name)
		if err := k.kc.Pods(pod.Namespace).Delete(pod.Name, deleteOptions); err != nil {
			glog.Errorf("failed terminating pod %q: $v", pod.Name, err)
			// Continue anyways, the reboot should terminate it
		}
	}

	glog.Info("Node drained, rebooting")

	// reboot
	k.lc.Reboot(false)

	// cross fingers
	time.Sleep(24 * 7 * time.Hour)

	return nil
}

func (k *Klocksmith) watchUpdateStatus(update func(s updateengine.Status)) {
	glog.Info("Beginning to watch update_engine status")

	oldOperation := ""
	ch := make(chan updateengine.Status, 1)
	stop := make(chan struct{})

	go k.ue.ReceiveStatuses(ch, stop)

	// synthesize first message
	st, err := k.ue.GetStatus()
	if err != nil {
		glog.Fatalf("Failed to get update_engine status: %v", err)
	}

	ch <- st

	for status := range ch {
		if status.CurrentOperation != oldOperation && update != nil {
			update(status)
			oldOperation = status.CurrentOperation
		}
	}
}

// waitForRebootSignal watches update_engine and waits for
// updateengine.UpdateStatusUpdatedNeedReboot
func (k *Klocksmith) waitForRebootSignal() error {
	// first, check current status.
	status, err := k.ue.GetStatus()
	if err != nil {
		return fmt.Errorf("failed to get update_engine status: %v", err)
	}

	// if we're not in UpdateStatusUpdatedNeedReboot, then wait for it.
	if status.CurrentOperation != updateengine.UpdateStatusUpdatedNeedReboot {
		ctx, cancel := context.WithCancel(context.Background())
		stc := make(chan updateengine.Status)
		go k.ue.RebootNeededSignal(stc, ctx.Done())
		<-stc
		cancel()
	}

	// we are now in UpdateStatusUpdatedNeedReboot state.
	return nil
}

// waitForOkToReboot waits for both 'ok-to-reboot' and 'needs-reboot' to be true.
func (k *Klocksmith) waitForOkToReboot() error {
	n, err := k.nc.Get(k.node, v1meta.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get self node (%q): %v", k.node, err)
	}

	if n.Annotations[constants.AnnotationOkToReboot] == constants.True && n.Annotations[constants.AnnotationRebootNeeded] == constants.True {
		return nil
	}

	// XXX: set timeout > 0?
	watcher, err := k.nc.Watch(v1meta.ListOptions{
		FieldSelector:   fields.OneTermEqualSelector("metadata.name", n.Name).String(),
		ResourceVersion: n.ResourceVersion,
	})
	if err != nil {
		return fmt.Errorf("failed to watch self node (%q): %v", k.node, err)
	}

	// hopefully 24 hours is enough time between indicating we need a
	// reboot and the controller telling us to do it
	ev, err := watch.Until(time.Hour*24, watcher, k8sutil.NodeAnnotationCondition(shouldRebootSelector))
	if err != nil {
		return fmt.Errorf("waiting for annotation %q failed: %v", constants.AnnotationOkToReboot, err)
	}

	// sanity check
	no, ok := ev.Object.(*v1.Node)
	if !ok {
		panic("event contains a non-*api.Node object")
	}

	if no.Annotations[constants.AnnotationOkToReboot] != constants.True {
		panic("event did not contain annotation expected")
	}

	return nil
}

func (k *Klocksmith) waitForNotOkToReboot() error {
	n, err := k.nc.Get(k.node, v1meta.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get self node (%q): %v", k.node, err)
	}

	if n.Annotations[constants.AnnotationOkToReboot] != constants.True {
		return nil
	}

	// XXX: set timeout > 0?
	watcher, err := k.nc.Watch(v1meta.ListOptions{
		FieldSelector:   fields.OneTermEqualSelector("metadata.name", n.Name).String(),
		ResourceVersion: n.ResourceVersion,
	})
	if err != nil {
		return fmt.Errorf("failed to watch self node (%q): %v", k.node, err)
	}

	// Within 24 hours of indicating we don't need a reboot we should be given a not-ok.
	// If that isn't the case, it likely means the operator isn't running, and
	// we'll just crash-loop in that case, and hopefully that will help the user realize something's wrong.
	// Use a custom condition function to use the more correct 'OkToReboot !=
	// true' vs '== False'; due to the operator matching on '== True', and not
	// going out of its way to convert '' => 'False', checking the exact inverse
	// of what the operator checks is the correct thing to do.
	ev, err := watch.Until(time.Hour*24, watcher, watch.ConditionFunc(func(event watch.Event) (bool, error) {
		switch event.Type {
		case watch.Error:
			return false, fmt.Errorf("error watching node: %v", event.Object)
		case watch.Deleted:
			return false, fmt.Errorf("our node was deleted while we were waiting for ready")
		}

		no := event.Object.(*v1.Node)
		if no.Annotations[constants.AnnotationOkToReboot] != constants.True {
			return true, nil
		}
		return false, nil
	}))
	if err != nil {
		return fmt.Errorf("waiting for annotation %q failed: %v", constants.AnnotationOkToReboot, err)
	}

	// sanity check
	no := ev.Object.(*v1.Node)

	if no.Annotations[constants.AnnotationOkToReboot] == constants.True {
		panic("event did not contain annotation expected")
	}

	return nil
}

func (k *Klocksmith) getPodsForDeletion() ([]v1.Pod, error) {
	pods, err := drain.GetPodsForDeletion(k.kc, k.node)
	if err != nil {
		return nil, fmt.Errorf("failed to get list of pods for deletion: %v", err)
	}

	// XXX: ignoring kube-system is a simple way to avoid eviciting
	// critical components such as kube-scheduler and
	// kube-controller-manager.

	pods = k8sutil.FilterPods(pods, func(p *v1.Pod) bool {
		if p.Namespace == "kube-system" {
			return false
		}

		return true
	})

	return pods, nil
}
