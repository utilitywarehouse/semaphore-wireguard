package kube

import (
	"context"
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

type patch struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value string `json:"value"`
}

// PatchNodeAnnotation will send a node patch request for the annotation path.
// Since nodes are updated every 10seconds by default:
// https://github.com/kubernetes/client-go/issues/414 using Update will have the
// risk of failing
func PatchNodeAnnotation(client kubernetes.Interface, nodeName string, annotations map[string]string) error {
	ctx := context.Background()
	patchData := map[string]interface{}{
		"metadata": map[string]map[string]string{
			"annotations": annotations,
		},
	}
	payloadBytes, err := json.Marshal(patchData)
	if err != nil {
		return err
	}
	_, err = client.CoreV1().Nodes().Patch(ctx, nodeName, types.StrategicMergePatchType, payloadBytes, metav1.PatchOptions{})
	return err
}
