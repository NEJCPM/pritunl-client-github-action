package provisioner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NEJCPM/pritunl-client-github-action/pkg/domain"
)

func sha256File(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestLinuxProvisioner_ChecksumVerifiedBeforeInstall(t *testing.T) {
	runner := newFakeRunner()
	l := newTestLinux(runner)

	temp := t.TempDir()
	artifact := filepath.Join(temp, "pritunl-client.deb")
	if err := os.WriteFile(artifact, []byte("fake deb payload"), 0600); err != nil {
		t.Fatal(err)
	}

	digest := sha256File(t, artifact)
	var verifiedPath string
	l.verify = func(cfg domain.ActionConfig, artifactKey string, path string) error {
		verifiedPath = path
		if artifactKey != "deb-jammy-amd64" {
			t.Errorf("artifactKey = %q, want deb-jammy-amd64", artifactKey)
		}
		return verifyWithDigest(path, digest)
	}

	cfg := domain.ActionConfig{ClientVersion: "1.2.3.4", RunnerTemp: temp}
	if err := l.Provision(context.Background(), cfg); err != nil {
		t.Fatalf("Provision failed: %v", err)
	}
	if verifiedPath != artifact {
		t.Errorf("verified path = %q, want %q", verifiedPath, artifact)
	}
}

func TestLinuxProvisioner_ChecksumMismatchAbortsInstall(t *testing.T) {
	runner := newFakeRunner()
	l := newTestLinux(runner)

	temp := t.TempDir()
	l.verify = func(cfg domain.ActionConfig, artifactKey string, path string) error {
		return fmt.Errorf("checksum mismatch for %s", path)
	}

	err := l.Provision(context.Background(), domain.ActionConfig{ClientVersion: "1.2.3.4", RunnerTemp: temp})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum failure, got: %v", err)
	}
	if len(runner.all("sudo")) > 3 {
		t.Error("apt-get install of the deb must not run after checksum failure")
	}
}

func TestLinuxProvisioner_MissingChecksumAborts(t *testing.T) {
	runner := newFakeRunner()
	l := newTestLinux(runner)
	l.verify = func(cfg domain.ActionConfig, artifactKey string, path string) error {
		return fmt.Errorf("version 1.2.3.4 has no pinned checksum for %q", artifactKey)
	}

	err := l.Provision(context.Background(), domain.ActionConfig{ClientVersion: "1.2.3.4", RunnerTemp: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "no pinned checksum") {
		t.Fatalf("expected missing checksum error, got: %v", err)
	}
}

func TestMacOSProvisioner_ChecksumVerifiedBeforeUnzip(t *testing.T) {
	runner := newFakeRunner()
	m := newTestMacOS(runner)
	t.Setenv("HOME", t.TempDir())

	var order []string
	m.verify = func(cfg domain.ActionConfig, artifactKey string, path string) error {
		if artifactKey != "pkg-zip" {
			t.Errorf("artifactKey = %q, want pkg-zip", artifactKey)
		}
		order = append(order, "verify:"+filepath.Base(path))
		return nil
	}
	runner.onCall = append(runner.onCall, func(name string, args []string) {
		order = append(order, "cmd:"+name)
	})

	cfg := domain.ActionConfig{ClientVersion: "1.2.3.4", RunnerTemp: t.TempDir()}
	if err := m.Provision(context.Background(), cfg); err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	joined := strings.Join(order, ",")
	if !strings.Contains(joined, "verify:Pritunl-") {
		t.Errorf("verify did not run: %s", joined)
	}
	if strings.Index(joined, "verify:") > strings.Index(joined, "cmd:unzip") {
		t.Errorf("verify must run before unzip: %s", joined)
	}
}

func TestWindowsProvisioner_ChecksumVerifiedBeforeInstaller(t *testing.T) {
	runner := newFakeRunner()
	w := newTestWindows(runner)

	var order []string
	w.verify = func(cfg domain.ActionConfig, artifactKey string, path string) error {
		if artifactKey != "exe" {
			t.Errorf("artifactKey = %q, want exe", artifactKey)
		}
		order = append(order, "verify:"+filepath.Base(path))
		return nil
	}
	runner.onCall = append(runner.onCall, func(name string, args []string) {
		order = append(order, "cmd:"+name)
	})

	cfg := domain.ActionConfig{ClientVersion: "1.2.3.4", RunnerTemp: t.TempDir()}
	if err := w.Provision(context.Background(), cfg); err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	joined := strings.Join(order, ",")
	if strings.Index(joined, "verify:") > strings.Index(joined, "cmd:pwsh") {
		t.Errorf("verify must run before the installer: %s", joined)
	}
}

// verifyWithDigest hashes filePath and compares it with digest.
func verifyWithDigest(filePath, digest string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != digest {
		return fmt.Errorf("checksum mismatch for %s", filePath)
	}
	return nil
}
