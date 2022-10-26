package oci

import (
	"context"
	"fmt"

	"github.com/fluxcd/pkg/oci/client"
	"github.com/google/go-containerregistry/pkg/crane"
)

type Client struct {
	client *client.Client
}

// NewClient creates a new OCI client with target URL and user agent.
func NewClient(agent string) *Client {
	options := []crane.Option{
		crane.WithUserAgent(agent),
	}
	client := client.NewClient(options)

	return &Client{
		client: client,
	}
}

// Pull takes a snapshot name and pulls it from the OCI repository.
func (o *Client) Pull(ctx context.Context, url, outDir string) (string, error) {
	m, err := o.client.Pull(ctx, url, outDir)
	if err != nil {
		return "", fmt.Errorf("failed to pull snapshot: %w", err)
	}

	return m.Digest, nil
}

// Push takes a path, creates an archive of the files in it and pushes the content to the OCI registry.
func (o *Client) Push(ctx context.Context, url, artifactPath, sourcePath string, metadata client.Metadata) (string, error) {
	if err := o.client.Build(artifactPath, sourcePath, nil); err != nil {
		return "", fmt.Errorf("failed to create archive of the fetched artifacts: %w", err)
	}
	digest, err := o.client.Push(ctx, url, sourcePath, metadata, nil)
	if err != nil {
		return "", fmt.Errorf("failed to push oci image: %w", err)
	}
	return digest, nil
}
