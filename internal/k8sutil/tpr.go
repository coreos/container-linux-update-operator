package k8sutil

import (
	"fmt"
	"time"

	"k8s.io/client-go/1.5/kubernetes"
	v1api "k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/1.5/pkg/util/wait"
)

func CreateTPR(kc *kubernetes.Clientset, name, version, desc string) error {
	tpri := kc.ThirdPartyResources()
	tpr := &v1beta1.ThirdPartyResource{
		ObjectMeta: v1api.ObjectMeta{
			Name: name,
		},
		Versions: []v1beta1.APIVersion{
			{Name: version},
		},
		Description: desc,
	}

	_, err := tpri.Create(tpr)
	if err != nil {
		if IsKubernetesResourceAlreadyExistError(err) {
			return nil
		}

		return fmt.Errorf("failed creating ThirdPartyResource: %v", err)
	}

	if err := wait.PollImmediate(5*time.Second, 30*time.Second, func() (bool, error) {
		_, err = tpri.Get(name)
		if err != nil {
			return false, err
		}

		return true, nil
	}); err != nil {
		return fmt.Errorf("failed waiting for ThirdPartyResource in API server: %v", err)
	}

	return nil
}
