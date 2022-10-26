package pkg

import (
	"context"

	"github.com/fluxcd/pkg/oci/client"
)

// OCIClient defines the needed capabilities of a client that can interact with an OCI repository.
type OCIClient interface {
	Pull(ctx context.Context, url, outDir string) (string, error)
	Push(ctx context.Context, url, artifactPath, sourcePath string, metadata client.Metadata) (string, error)
}
