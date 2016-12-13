package k8sutil

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	v1core "k8s.io/client-go/1.5/kubernetes/typed/core/v1"
	v1api "k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/watch"
)

const (
	updateConfPath         = "/usr/share/coreos/update.conf"
	updateConfOverridePath = "/etc/coreos/update.conf"
	osReleasePath          = "/etc/os-release"
)

// NodeLabelCondition returns a condition function that succeeds when a node
// being watched has a label of key equal to value.
func NodeLabelCondition(key, value string) watch.ConditionFunc {
	return func(event watch.Event) (bool, error) {
		switch event.Type {
		case watch.Modified:
			node := event.Object.(*v1api.Node)
			if node.Labels[key] == value {
				return true, nil
			}

			return false, nil
		}

		return false, fmt.Errorf("unhandled watch case for %#v", event)
	}
}

// updateNodeRetry calls f to update a node object in Kubernetes. f is called
// once and expected to change the spec or metadata of the node argument.
// updateNodeRetry will retry the update if there is a conflict using
// DefaultBackoff.
func updateNodeRetry(nc v1core.NodeInterface, node string, f func(*v1api.Node)) error {
	n, err := nc.Get(node)
	if err != nil {
		return fmt.Errorf("failed to get node %q: %v", node, err)
	}

	f(n)

	err = RetryOnConflict(DefaultBackoff, func() (err error) {
		n, err = nc.Update(n)
		return
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
	return updateNodeRetry(nc, node, func(n *v1api.Node) {
		for k, v := range m {
			n.Labels[k] = v
		}
	})
}

// SetNodeAnnotations sets all keys in m to their respective values in
// node's annotations.
func SetNodeAnnotations(nc v1core.NodeInterface, node string, m map[string]string) error {
	return updateNodeRetry(nc, node, func(n *v1api.Node) {
		for k, v := range m {
			n.Annotations[k] = v
		}
	})
}

// Unschedulable marks node as schedulable or unschedulable according to sched.
func Unschedulable(nc v1core.NodeInterface, node string, sched bool) error {
	n, err := nc.Get(node)
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
		log.Printf("Skipping missing update.conf: %v", err)
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
