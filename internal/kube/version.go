package kube

import (
	"context"
	"fmt"
)

// ServerVersion returns the Kubernetes server version string
func (c *Client) ServerVersion(ctx context.Context) (string, error) {
	info, err := c.Clientset.Discovery().ServerVersion()
	if err != nil {
		return "", fmt.Errorf("kube: failed to get server version: %w", err)
	}
	return info.GitVersion, nil
}
