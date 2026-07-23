package cli

import (
	"context"

	"github.com/nathanielvarona/pritunl-client-github-action/pkg/domain"
)

// PritunlCLI defines the seam for interacting with the pritunl-client binary.
type PritunlCLI interface {
	Version(ctx context.Context) (string, error)
	AddProfile(ctx context.Context, tarPath string) error
	ListServers(ctx context.Context) ([]domain.ProfileServer, error)
	StartConnection(ctx context.Context, serverID string, mode string, password string) error
}
