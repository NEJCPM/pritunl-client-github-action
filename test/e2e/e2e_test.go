//go:build e2e

package e2e

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/NEJCPM/pritunl-client-github-action/pkg/cli"
	"github.com/NEJCPM/pritunl-client-github-action/pkg/domain"
	"github.com/NEJCPM/pritunl-client-github-action/pkg/engine"
	"github.com/NEJCPM/pritunl-client-github-action/pkg/provisioner"
)

const (
	testOrgName    = "e2e-org"
	testUserName   = "e2e-user"
	testServerName = "e2e-server"
	testUserPIN    = "1234567890"
	testNetwork    = "10.220.0.0/24"
	serverURL      = "https://localhost"
)

// composeFile resolves docker-compose.test.yml whether tests run from the
// test/e2e directory or from the repository root.
func composeFile(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"docker-compose.test.yml",
		filepath.Join("test", "e2e", "docker-compose.test.yml"),
		filepath.Join("..", "e2e", "docker-compose.test.yml"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	t.Fatal("docker-compose.test.yml not found")
	return ""
}

// ensureStack starts the Pritunl/Mongo test stack via docker compose.
func ensureStack(t *testing.T) {
	t.Helper()
	compose := composeFile(t)
	cmd := exec.Command("docker", "compose", "-f", compose, "up", "-d", "--wait")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker compose up failed: %v\n%s", err, truncate(string(out), 2000))
	}
}

// teardownStack removes the stack and its volumes.
func teardownStack(t *testing.T) {
	t.Helper()
	compose := composeFile(t)
	cmd := exec.Command("docker", "compose", "-f", compose, "down", "-v")
	out, _ := cmd.CombinedOutput()
	_ = out
}

// dumpStackLogs prints sanitized server logs for post-mortem on failure.
func dumpStackLogs(t *testing.T) {
	t.Helper()
	compose := composeFile(t)
	cmd := exec.Command("docker", "compose", "-f", compose, "logs", "--tail", "60", "pritunl")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return
	}
	t.Logf("Pritunl server logs (sanitized):\n%s", sanitizeLogs(string(out)))
}

// sanitizeLogs strips obvious credential patterns before printing.
func sanitizeLogs(logs string) string {
	lines := strings.Split(logs, "\n")
	var kept []string
	for _, line := range lines {
		if strings.Contains(line, "password") || strings.Contains(line, "secret") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func defaultCredentials(t *testing.T) (string, string, error) {
	t.Helper()
	compose := composeFile(t)
	cmd := exec.Command("docker", "compose", "-f", compose, "exec", "-T", "pritunl", "pritunl", "default-password")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("default-password command failed: %w: %s", err, strings.TrimSpace(string(out)))
	}

	username := regexp.MustCompile(`username: "([^"]+)"`).FindStringSubmatch(string(out))
	password := regexp.MustCompile(`password: "([^"]+)"`).FindStringSubmatch(string(out))
	if len(username) != 2 || len(password) != 2 {
		return "", "", fmt.Errorf("could not parse default credentials")
	}
	return username[1], password[1], nil
}

// bootstrapTestServer authenticates and provisions the org/user/server trio.
func bootstrapTestServer(t *testing.T) (orgID, userID, serverID string) {
	t.Helper()
	ensureStack(t)

	username, password, err := defaultCredentials(t)
	if err != nil {
		t.Fatalf("failed to read default credentials: %v", err)
	}

	client, err := NewPritunlAPIClient(serverURL)
	if err != nil {
		t.Fatalf("failed to create API client: %v", err)
	}
	if err := client.Authenticate(username, password); err != nil {
		t.Fatalf("failed to authenticate with Pritunl server: %v", err)
	}

	orgID, err = client.CreateOrganization(testOrgName)
	if err != nil {
		t.Fatalf("failed to create organization: %v", err)
	}
	userID, err = client.CreateUser(orgID, testUserName, testUserPIN)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	serverID, err = client.CreateServer(testServerName, testNetwork)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	if err := client.AttachOrganization(serverID, orgID); err != nil {
		t.Fatalf("failed to attach organization: %v", err)
	}
	if err := client.WaitForServerOnline(serverID, 4*time.Minute); err != nil {
		dumpStackLogs(t)
		t.Fatalf("server did not come online: %v", err)
	}

	return orgID, userID, serverID
}

// buildProfileFixture downloads the user profile and points the remote
// endpoint at the loopback port-mapping of the test container.
func buildProfileFixture(t *testing.T, orgID, userID string) (profileBase64 string) {
	t.Helper()

	username, password, err := defaultCredentials(t)
	if err != nil {
		t.Fatalf("failed to read default credentials: %v", err)
	}
	client, err := NewPritunlAPIClient(serverURL)
	if err != nil {
		t.Fatalf("failed to create API client: %v", err)
	}
	if err := client.Authenticate(username, password); err != nil {
		t.Fatalf("failed to authenticate for profile download: %v", err)
	}

	rawTar, err := client.GetUserKeyTar(orgID, userID)
	if err != nil {
		t.Fatalf("failed to download user profile: %v", err)
	}
	rewritten, err := RewriteProfileRemoteHost(rawTar, "127.0.0.1")
	if err != nil {
		t.Fatalf("failed to rewrite profile remote: %v", err)
	}

	return base64.StdEncoding.EncodeToString(rewritten)
}

// TestE2E_PritunlServerAuth verifies the compose stack authenticates.
func TestE2E_PritunlServerAuth(t *testing.T) {
	ensureStack(t)

	username, password, err := defaultCredentials(t)
	if err != nil {
		t.Fatalf("failed to read default credentials: %v", err)
	}

	client, err := NewPritunlAPIClient(serverURL)
	if err != nil {
		t.Fatalf("failed to create API client: %v", err)
	}
	if err := client.Authenticate(username, password); err != nil {
		t.Fatalf("failed to authenticate with default credentials: %v", err)
	}

	if err := client.refreshCSRF(); err != nil {
		t.Fatalf("state refresh failed: %v", err)
	}
	t.Log("Successfully authenticated with Pritunl server")
}

// TestE2E_ProfileFixture generates a valid, connectable profile tar.
// When E2E_PROFILE_OUT is set, the base64 fixture is written to that path
// (never logged) so workflows can feed it to the packaged composite action.
func TestE2E_ProfileFixture(t *testing.T) {
	orgID, userID, _ := bootstrapTestServer(t)
	profile := buildProfileFixture(t, orgID, userID)
	if len(profile) < 1024 {
		t.Fatalf("profile fixture suspiciously small: %d bytes", len(profile))
	}
	if out := os.Getenv("E2E_PROFILE_OUT"); out != "" {
		if err := os.WriteFile(out, []byte(profile), 0600); err != nil {
			t.Fatalf("failed to write profile fixture: %v", err)
		}
	}
}

// TestE2E_FullActionFlowLinux runs the complete production path: real Linux
// provisioning (apt + GPG keyring), profile import, connection start and
// tunnel validation, against a real Pritunl server in Docker.
func TestE2E_FullActionFlowLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("full flow requires a Linux runner (CI runs this on ubuntu-24.04); got %s", runtime.GOOS)
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		t.Skip("full flow cannot run inside a container host setup")
	}

	orgID, userID, _ := bootstrapTestServer(t)
	profileBase64 := buildProfileFixture(t, orgID, userID)

	tempDir := t.TempDir()
	cfg := domain.ActionConfig{
		ProfileFile:                  profileBase64,
		ProfilePin:                   testUserPIN,
		VPNMode:                      "ovpn",
		ClientVersion:                "", // package manager path (apt + GPG keyring)
		StartConnection:              true,
		ReadyProfileTimeout:          60,
		EstablishedConnectionTimeout: 120,
		ConcealedOutputs:             true,
		RunnerOS:                     "Linux",
		RunnerTemp:                   tempDir,
		GitHubOutput:                 filepath.Join(tempDir, "github_output"),
		GitHubActions:                true,
	}

	// 1. Real provisioning path: key download, fingerprint check, keyring,
	//    signed-by repo, apt-get update/install.
	prov, err := provisioner.NewProvisioner("Linux")
	if err != nil {
		t.Fatalf("provisioner init failed: %v", err)
	}
	provCtx, provCancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer provCancel()
	if err := prov.Provision(provCtx, cfg); err != nil {
		dumpStackLogs(t)
		t.Fatalf("provisioning failed: %v", err)
	}

	// 2. Real connection lifecycle through the engine.
	systemCLI := cli.NewSystemCLI("")
	vpnEngine := engine.NewEngine(systemCLI)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	result, err := vpnEngine.Connect(ctx, cfg)
	if err != nil {
		dumpStackLogs(t)
		t.Fatalf("engine connect failed: %v", err)
	}

	if result.PrimaryClientID == "" {
		t.Fatal("engine produced empty primary client id")
	}
	if result.ConnectedCount != result.ExpectedCount {
		t.Fatalf("expected %d connected servers, got %d", result.ExpectedCount, result.ConnectedCount)
	}

	// 3. Validate the tunnel: the client must hold a valid tunnel address
	//    inside the test network.
	servers, err := systemCLI.ListServers(ctx)
	if err != nil {
		t.Fatalf("failed to list servers after connect: %v", err)
	}
	var tunnelIP string
	for _, s := range servers {
		if s.ID == result.PrimaryClientID && s.ClientAddress != "" {
			tunnelIP = s.ClientAddress
		}
	}
	if tunnelIP == "" {
		t.Fatal("primary client has no tunnel address after connection")
	}
	ip, _, err := net.ParseCIDR(tunnelIP)
	if err != nil {
		ip = net.ParseIP(tunnelIP)
	}
	if ip == nil {
		t.Fatalf("tunnel address %q is not a valid IP/CIDR", tunnelIP)
	}
	t.Logf("tunnel established: primary=%s address=%s", result.PrimaryClientID, tunnelIP)

	// 4. Validate GITHUB_OUTPUT contents.
	outputData, err := os.ReadFile(cfg.GitHubOutput)
	if err != nil {
		t.Fatalf("failed to read action outputs: %v", err)
	}
	outStr := string(outputData)
	if !strings.Contains(outStr, "client-id="+result.PrimaryClientID) {
		t.Errorf("GITHUB_OUTPUT missing client-id; got: %s", outStr)
	}
	if !strings.Contains(outStr, "client-ids=") {
		t.Errorf("GITHUB_OUTPUT missing client-ids")
	}

	// 5. Stop the connection.
	stopCmd := exec.Command("pritunl-client", "stop", result.PrimaryClientID)
	if out, stopErr := stopCmd.CombinedOutput(); stopErr != nil {
		t.Logf("stop connection warning: %v: %s", stopErr, truncate(string(out), 200))
	}
}
