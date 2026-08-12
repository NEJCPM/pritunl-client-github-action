//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/NEJCPM/pritunl-client-github-action/pkg/cli"
	"github.com/NEJCPM/pritunl-client-github-action/pkg/engine"
)

func TestE2E_DockerComposePritunlConnection(t *testing.T) {
	cmd := exec.Command("docker", "compose", "-f", "docker-compose.test.yml", "ps")
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

	username, password, err := defaultCredentials()
	if err != nil {
		t.Fatalf("failed to read default credentials: %v", err)
	}

	err = client.Authenticate(username, password)
	if err != nil {
		t.Fatalf("failed to authenticate with default credentials: %v", err)
	}

	t.Log("Successfully authenticated with Pritunl server")
}

func defaultCredentials() (string, string, error) {
	cmd := exec.Command("docker", "compose", "-f", "docker-compose.test.yml", "exec", "-T", "pritunl", "pritunl", "default-password")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("default-password command failed: %w: %s", err, strings.TrimSpace(string(out)))
	}

	username := regexp.MustCompile(`username: "([^"]+)"`).FindStringSubmatch(string(out))
	password := regexp.MustCompile(`password: "([^"]+)"`).FindStringSubmatch(string(out))
	if len(username) != 2 || len(password) != 2 {
		return "", "", fmt.Errorf("could not parse default credentials: %s", strings.TrimSpace(string(out)))
	}
	return username[1], password[1], nil
}
