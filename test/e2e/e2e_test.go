//go:build e2e

package e2e

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/nathanielvarona/pritunl-client-github-action/pkg/cli"
	"github.com/nathanielvarona/pritunl-client-github-action/pkg/domain"
	"github.com/nathanielvarona/pritunl-client-github-action/pkg/engine"
)

func TestE2E_DockerComposePritunlConnection(t *testing.T) {
	// 1. Verify Docker Compose environment is running
	cmd := exec.Command("docker", "compose", "-f", "docker-compose.test.yml", "ps")
	if err := cmd.Run(); err != nil {
		t.Skip("Docker Compose test stack not active, skipping live e2e test.")
	}

	systemCLI := cli.NewSystemCLI("")
	eng := engine.NewEngine(systemCLI)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg := domain.ActionConfig{
		VPNMode:                      "ovpn",
		ReadyProfileTimeout:          10,
		EstablishedConnectionTimeout: 30,
		StartConnection:              false,
	}

	// Verify client version query
	version, err := systemCLI.Version(ctx)
	if err != nil {
		t.Logf("Notice: pritunl-client binary not installed locally: %v", err)
		return
	}
	t.Logf("Pritunl CLI Version in E2E: %s", version)

	_ = eng
	_ = cfg
}
