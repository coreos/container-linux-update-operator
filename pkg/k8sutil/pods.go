package k8sutil

import (
	v1api "k8s.io/api/core/v1"
)

func FilterPods(pods []v1api.Pod, filter func(*v1api.Pod) bool) (newpods []v1api.Pod) {
	for _, p := range pods {
		if filter(&p) {
			newpods = append(newpods, p)
		}
	}

	return
}
