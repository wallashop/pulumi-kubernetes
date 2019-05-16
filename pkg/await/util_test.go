package await

import (
	"context"
	"fmt"
	"testing"

	"github.com/pulumi/pulumi-kubernetes/pkg/await/fixtures"
	"github.com/pulumi/pulumi-kubernetes/pkg/logging"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TODO: move this
func mockAwaitConfig(inputs *unstructured.Unstructured) createAwaitConfig {
	return createAwaitConfig{
		ctx: context.Background(),
		//TODO: complete this mock if needed
		currentInputs:  inputs,
		currentOutputs: inputs,
		logger:         logging.NewLogger(context.Background(), nil, ""),
	}
}

// TODO: move this
func decodeUnstructured(text string) (*unstructured.Unstructured, error) {
	obj, _, err := unstructured.UnstructuredJSONScheme.Decode([]byte(text), nil, nil)
	if err != nil {
		return nil, err
	}
	unst, isUnstructured := obj.(*unstructured.Unstructured)
	if !isUnstructured {
		return nil, fmt.Errorf("could not decode object as *unstructured.Unstructured: %v", unst)
	}
	return unst, nil
}

func TestFqName(t *testing.T) {
	pod := fixtures.PodBasic()
	podNoNS := pod.Object.DeepCopy()
	podNoNS.Namespace = ""
	podFooNS := pod.Object.DeepCopy()
	podFooNS.Namespace = "foo"

	type args struct {
		d metav1.Object
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"default-ns", args{d: metav1.Object(pod.Object)}, pod.Object.Name},
		{"no-ns", args{d: metav1.Object(podNoNS)}, podNoNS.Name},
		{"foo-ns", args{d: metav1.Object(podFooNS)},
			fmt.Sprintf("%s/%s", podFooNS.Namespace, podFooNS.Name)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fqName(tt.args.d); got != tt.want {
				t.Errorf("fqName() = %v, want %v", got, tt.want)
			}
		})
	}
}
