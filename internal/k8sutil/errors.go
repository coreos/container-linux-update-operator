package k8sutil

import (
	"net/http"

	"k8s.io/client-go/1.5/pkg/api/errors"
	"k8s.io/client-go/1.5/pkg/api/unversioned"
)

func IsKubernetesResourceAlreadyExistError(err error) bool {
	se, ok := err.(*errors.StatusError)
	if !ok {
		return false
	}

	if se.Status().Code == http.StatusConflict && se.Status().Reason == unversioned.StatusReasonAlreadyExists {
		return true
	}

	return false
}
