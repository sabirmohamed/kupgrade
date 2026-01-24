package kube

import (
	"fmt"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
)

// Client wraps the Kubernetes clientset and informer factory
type Client struct {
	Clientset *kubernetes.Clientset
	Factory   informers.SharedInformerFactory
	Context   string
	Namespace string
}

// NewClient creates a new Kubernetes client from ConfigFlags
func NewClient(configFlags *genericclioptions.ConfigFlags) (*Client, error) {
	restConfig, err := configFlags.ToRESTConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create rest config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	// Get namespace from flags (empty string means all namespaces)
	namespace := ""
	if configFlags.Namespace != nil && *configFlags.Namespace != "" {
		namespace = *configFlags.Namespace
	}

	// Get context name for display
	contextName := ""
	if configFlags.Context != nil && *configFlags.Context != "" {
		contextName = *configFlags.Context
	} else {
		// Try to get from raw config
		rawConfig, err := configFlags.ToRawKubeConfigLoader().RawConfig()
		if err == nil {
			contextName = rawConfig.CurrentContext
		}
	}

	// Create informer factory (0 = no resync for Phase 1)
	factory := informers.NewSharedInformerFactory(clientset, 0)

	return &Client{
		Clientset: clientset,
		Factory:   factory,
		Context:   contextName,
		Namespace: namespace,
	}, nil
}
