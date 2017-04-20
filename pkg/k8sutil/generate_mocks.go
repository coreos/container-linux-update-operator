package k8sutil

//go:generate mockgen -destination ./mocks/node_interface_mock.go k8s.io/client-go/kubernetes/typed/core/v1 NodeInterface
