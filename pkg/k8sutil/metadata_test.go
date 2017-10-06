package k8sutil

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/golang/mock/gomock"

	mock_v1 "github.com/coreos/container-linux-update-operator/pkg/k8sutil/mocks"
	v1api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func atomicCounterIncrement(n *v1api.Node) {
	counterAnno := "counter"
	s := n.Annotations[counterAnno]
	var i int
	if s == "" {
		i = 0
	} else {
		var err error
		i, err = strconv.Atoi(s)
		if err != nil {
			panic(err)
		}
	}
	n.Annotations[counterAnno] = strconv.Itoa(i + 1)
}

func TestUpdateNodeRetryHandlesConflict(t *testing.T) {
	DefaultBackoff.Duration = 0
	DefaultRetry.Duration = 0

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockNi := mock_v1.NewMockNodeInterface(ctrl)
	mockNode := &v1api.Node{}
	mockNode.SetName("mock_node")
	mockNode.SetNamespace("default")
	mockNode.SetAnnotations(map[string]string{"counter": "20"})
	mockNode.SetResourceVersion("20")

	mockNi.EXPECT().Get("mock_node", v1meta.GetOptions{}).Return(mockNode, nil).AnyTimes()

	// Conflict once; mock that a third party incremented the counter from '20'
	// to '21' right after the node is returned
	gomock.InOrder(
		mockNi.EXPECT().Update(mockNode).Do(func(n *v1api.Node) {
			// Fake conflict; the counter was incremented elsewhere; resourceVersion is now 21
			mockNode.SetAnnotations(map[string]string{"counter": "21"})
			mockNode.SetResourceVersion("21")
		}).Return(mockNode, errors.NewConflict(schema.GroupResource{}, "mock_node", fmt.Errorf("err"))),

		// And then the successful retry
		mockNi.EXPECT().Update(mockNode).Return(mockNode, nil),
	)

	err := UpdateNodeRetry(mockNi, "mock_node", atomicCounterIncrement)

	if err != nil {
		t.Errorf("unexpected error: expected increment to succeed")
	}
	if mockNode.Annotations["counter"] != "22" {
		t.Errorf("expected the counter to hit 22; was %v", mockNode.Annotations["counter"])
	}
}
