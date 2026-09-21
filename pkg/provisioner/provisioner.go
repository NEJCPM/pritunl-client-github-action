package provisioner

import (
	"context"
	"fmt"

	"github.com/NEJCPM/pritunl-client-github-action/pkg/domain"
)

// commandRunner is the seam used by provisioners to execute system commands.
type commandRunner func(ctx context.Context, name string, args ...string) error

// PlatformProvisioner handles OS package manager installation of Pritunl Client and dependencies.
type PlatformProvisioner interface {
	Provision(ctx context.Context, cfg domain.ActionConfig) error
}

// NewProvisioner returns the appropriate provisioner for the given runner OS.
func NewProvisioner(runnerOS string) (PlatformProvisioner, error) {
	switch runnerOS {
	case "Linux":
		return NewLinuxProvisioner(), nil
	case "macOS", "Darwin":
		return NewMacOSProvisioner(), nil
	case "Windows":
		return NewWindowsProvisioner(), nil
	default:
		return nil, fmt.Errorf("unsupported runner operating system: %s", runnerOS)
	}
}
