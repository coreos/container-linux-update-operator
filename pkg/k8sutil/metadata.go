package k8sutil

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/golang/glog"
	v1api "k8s.io/api/core/v1"
	v1meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/watch"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
)

const (
	updateConfPath         = "/usr/share/coreos/update.conf"
	updateConfOverridePath = "/etc/coreos/update.conf"
	osReleasePath          = "/etc/os-release"
)

// NodeAnnotationCondition returns a condition function that succeeds when a
// node being watched has an annotation of key equal to value.
func NodeAnnotationCondition(selector fields.Selector) watch.ConditionFunc {
	return func(event watch.Event) (bool, error) {
		switch event.Type {
		case watch.Modified:
			node := event.Object.(*v1api.Node)
			return selector.Matches(fields.Set(node.Annotations)), nil
		}

		return false, fmt.Errorf("unhandled watch case for %#v", event)
	}
}

// UpdateNodeRetry calls f to update a node object in Kubernetes.
// It will attempt to update the node by applying f to it up to DefaultBackoff
// number of times.
// f will be called each time since the node object will likely have changed if
// a retry is necessary.
func UpdateNodeRetry(nc v1core.NodeInterface, node string, f func(*v1api.Node)) error {
	err := RetryOnConflict(DefaultBackoff, func() error {
		n, getErr := nc.Get(node, v1meta.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("failed to get node %q: %v", node, getErr)
		}

		f(n)

		_, err := nc.Update(n)
		return err
	})
	if err != nil {
		// may be conflict if max retries were hit
		return fmt.Errorf("unable to update node %q: %v", node, err)
	}

	return nil
}

// SetNodeLabels sets all keys in m to their respective values in
// node's labels.
func SetNodeLabels(nc v1core.NodeInterface, node string, m map[string]string) error {
	return UpdateNodeRetry(nc, node, func(n *v1api.Node) {
		for k, v := range m {
			n.Labels[k] = v
		}
	})
}

// SetNodeAnnotations sets all keys in m to their respective values in
// node's annotations.
func SetNodeAnnotations(nc v1core.NodeInterface, node string, m map[string]string) error {
	return UpdateNodeRetry(nc, node, func(n *v1api.Node) {
		for k, v := range m {
			n.Annotations[k] = v
		}
	})
}

// DeleteNodeLabels deletes all keys in ks
func DeleteNodeLabels(nc v1core.NodeInterface, node string, ks []string) error {
	return UpdateNodeRetry(nc, node, func(n *v1api.Node) {
		for _, k := range ks {
			delete(n.Labels, k)
		}
	})
}

// DeleteNodeAnnotations deletes all annotations with keys in ks
func DeleteNodeAnnotations(nc v1core.NodeInterface, node string, ks []string) error {
	return UpdateNodeRetry(nc, node, func(n *v1api.Node) {
		for _, k := range ks {
			delete(n.Annotations, k)
		}
	})
}

// Unschedulable marks node as schedulable or unschedulable according to sched.
func Unschedulable(nc v1core.NodeInterface, node string, sched bool) error {
	n, err := nc.Get(node, v1meta.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get node %q: %v", node, err)
	}

	n.Spec.Unschedulable = sched

	if err := RetryOnConflict(DefaultBackoff, func() (err error) {
		n, err = nc.Update(n)
		return
	}); err != nil {
		return fmt.Errorf("unable to set 'Unschedulable' property of node %q to %t: %v", node, sched, err)
	}

	return nil
}

// splits newline-delimited KEY=VAL pairs and update map
func splitNewlineEnv(m map[string]string, envs string) {
	sc := bufio.NewScanner(strings.NewReader(envs))
	for sc.Scan() {
		spl := strings.SplitN(sc.Text(), "=", 2)

		// just skip empty lines or lines without a value
		if len(spl) == 1 {
			continue
		}

		m[spl[0]] = spl[1]
	}
}

// VersionInfo contains CoreOS version and update information.
type VersionInfo struct {
	Name    string
	ID      string
	Group   string
	Version string
}

func getUpdateMap() (map[string]string, error) {
	infomap := map[string]string{}

	// this file should always be present on CoreOS
	uconf, err := os.Open(updateConfPath)
	if err != nil {
		return nil, err
	}

	b, err := ioutil.ReadAll(uconf)
	uconf.Close()
	if err != nil {
		return nil, err
	}

	splitNewlineEnv(infomap, string(b))

	// if present and readable, this file has overrides
	econf, err := os.Open(updateConfOverridePath)
	if err != nil {
		glog.Infof("Skipping missing update.conf: %v", err)
	}

	b, err = ioutil.ReadAll(econf)
	econf.Close()
	if err == nil {
		splitNewlineEnv(infomap, string(b))
	}

	return infomap, nil
}

func getReleaseMap() (map[string]string, error) {
	infomap := map[string]string{}

	// this file should always be present on CoreOS
	osrelease, err := os.Open(osReleasePath)
	if err != nil {
		return nil, err
	}

	defer osrelease.Close()
	b, err := ioutil.ReadAll(osrelease)
	osrelease.Close()
	if err != nil {
		return nil, err
	}

	splitNewlineEnv(infomap, string(b))

	return infomap, nil
}

// GetVersionInfo returns VersionInfo from the current CoreOS system.
//
// Should probably live in a different package.
func GetVersionInfo() (*VersionInfo, error) {
	updateconf, err := getUpdateMap()
	if err != nil {
		return nil, fmt.Errorf("unable to get update configuration: %v", err)
	}

	osrelease, err := getReleaseMap()
	if err != nil {
		return nil, fmt.Errorf("unable to get os release info: %v", err)
	}

	vi := &VersionInfo{
		Name:    osrelease["NAME"],
		ID:      osrelease["ID"],
		Group:   updateconf["GROUP"],
		Version: osrelease["VERSION"],
	}

	return vi, nil
}
