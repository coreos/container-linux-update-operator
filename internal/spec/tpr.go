package spec

import (
	"k8s.io/client-go/1.5/pkg/api/unversioned"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/util/intstr"
)

const RebootKind = "reboot-group.coreos.com/v1"

// Group represents a group of nodes in Kubernetes, and a reboot policy for those nodes.
type RebootGroup struct {
	unversioned.TypeMeta `json:",inline"`
	v1.ObjectMeta        `json:"metadata,omitempty"`
	Spec                 RebootGroupSpec `json:"spec"`
}

type RebootGroupSpec struct {
	// Paused indicates reboots in the Group are paused.
	Paused         bool                `json:"paused,omitempty"`
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
}
