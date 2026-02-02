package kube

import (
	"fmt"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/dynamic"
)

// NewDynamicClient creates a dynamic Kubernetes client from ConfigFlags.
func NewDynamicClient(configFlags *genericclioptions.ConfigFlags) (dynamic.Interface, error) {
	restConfig, err := configFlags.ToRESTConfig()
	if err != nil {
		return nil, fmt.Errorf("kube: dynamic rest config: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("kube: dynamic client: %w", err)
	}

	return dynamicClient, nil
}
