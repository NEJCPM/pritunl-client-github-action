package provisioner

import (
	"context"
	"strings"
	"testing"

	"github.com/NEJCPM/pritunl-client-github-action/pkg/domain"
)

func TestLinuxProvisioner_WrongKeyFingerprint_Aborts(t *testing.T) {
	runner := newFakeRunner()
	l := newTestLinux(runner)
	l.showKeys = func(context.Context, string) (string, error) {
		return "fpr:::::::::0000000000000000000000000000000000000000", nil
	}

	err := l.Provision(context.Background(), domain.ActionConfig{RunnerTemp: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "does not match expected fingerprint") {
		t.Fatalf("expected fingerprint mismatch error, got: %v", err)
	}
}

func TestLinuxProvisioner_ShowKeysFailure_Aborts(t *testing.T) {
	runner := newFakeRunner()
	l := newTestLinux(runner)
	l.showKeys = func(context.Context, string) (string, error) {
		return "", errBoom
	}

	err := l.Provision(context.Background(), domain.ActionConfig{RunnerTemp: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "failed to inspect downloaded pritunl GPG key") {
		t.Fatalf("expected showKeys failure error, got: %v", err)
	}
}

func TestLinuxProvisioner_InvalidCodename_Aborts(t *testing.T) {
	runner := newFakeRunner()
	l := newTestLinux(runner)
	l.lsbCodename = func(context.Context) (string, error) {
		return "malicious\nvalue", nil
	}

	err := l.Provision(context.Background(), domain.ActionConfig{RunnerTemp: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "invalid distro codename") {
		t.Fatalf("expected invalid codename error, got: %v", err)
	}
}

func TestLinuxProvisioner_UsesFailFastCurl(t *testing.T) {
	runner := newFakeRunner()
	l := newTestLinux(runner)

	if err := l.Provision(context.Background(), domain.ActionConfig{RunnerTemp: t.TempDir()}); err != nil {
		t.Fatalf("Provision failed: %v", err)
	}
	script := runner.joined()
	if !strings.Contains(script, "curl -fsSL "+pritunlGPGKeyURL) {
		t.Error("expected key download to use curl -fsSL")
	}
}

func TestLinuxProvisioner_Arm64_UniqueContainerName(t *testing.T) {
	runner := newFakeRunner()
	l := newTestLinux(runner)
	l.goArch = "arm64"

	if err := l.Provision(context.Background(), domain.ActionConfig{RunnerTemp: t.TempDir()}); err != nil {
		t.Fatalf("Provision failed: %v", err)
	}
	script := runner.joined()
	if !strings.Contains(script, "--name pritunl-extract-") {
		t.Error("expected unique container name prefix pritunl-extract-")
	}
	if strings.Contains(script, "--name pritunl-extract \n") {
		t.Error("expected container name to carry a unique suffix")
	}
}
