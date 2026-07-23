package provisioner

import (
	"context"
	"fmt"

	"github.com/nathanielvarona/pritunl-client-github-action/pkg/domain"
)

// PlatformProvisioner handles OS package manager installation of Pritunl Client and dependencies.
type PlatformProvisioner interface {
	Provision(ctx context.Context, cfg domain.ActionConfig) error
}

// NewProvisioner returns the appropriate provisioner for the given runner OS.
func NewProvisioner(runnerOS string) (PlatformProvisioner, error) {
	switch runnerOS {
	case "Linux":
		return &LinuxProvisioner{}, nil
	case "macOS", "Darwin":
		return &MacOSProvisioner{}, nil
	case "Windows":
		return &WindowsProvisioner{}, nil
	default:
		return nil, fmt.Errorf("unsupported runner operating system: %s", runnerOS)
	}
}
