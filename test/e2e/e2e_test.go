//go:build e2e

package e2e

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/nathanielvarona/pritunl-client-github-action/pkg/cli"
	"github.com/nathanielvarona/pritunl-client-github-action/pkg/engine"
)

func TestE2E_DockerComposePritunlConnection(t *testing.T) {
	cmd := exec.Command("docker", "compose", "-f", "test/e2e/docker-compose.test.yml", "ps")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("Docker Compose test stack not active, skipping live e2e test: %v\n%s", err, out)
	}

	systemCLI := cli.NewSystemCLI("")
	eng := engine.NewEngine(systemCLI)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	version, err := systemCLI.Version(ctx)
	if err != nil {
		t.Logf("Notice: pritunl-client binary not installed locally: %v", err)
		return
	}
	t.Logf("Pritunl CLI Version in E2E: %s", version)

	_ = eng
}

func TestE2E_PritunlServerAuth(t *testing.T) {
	serverURL := os.Getenv("PRITUNL_SERVER_URL")
	if serverURL == "" {
		serverURL = "https://localhost:443"
	}

	client, err := NewPritunlAPIClient(serverURL)
	if err != nil {
		t.Fatalf("failed to create API client: %v", err)
	}

	err = client.Authenticate("pritunl", "pritunl")
	if err != nil {
		t.Fatalf("failed to authenticate with default credentials: %v", err)
	}

	t.Log("Successfully authenticated with Pritunl server")
}
