package kube

import (
	"context"
	"fmt"
)

// GetServerVersion returns the Kubernetes server version string
func (c *Client) GetServerVersion(ctx context.Context) (string, error) {
	info, err := c.Clientset.Discovery().ServerVersion()
	if err != nil {
		return "", fmt.Errorf("failed to get server version: %w", err)
	}
	return info.GitVersion, nil
}
