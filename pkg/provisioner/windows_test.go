package provisioner

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/NEJCPM/pritunl-client-github-action/pkg/domain"
)

var errBoom = errors.New("boom")

func newTestWindows(runner *fakeRunner) *WindowsProvisioner {
	w := NewWindowsProvisioner()
	w.run = runner.run
	return w
}

func TestWindowsProvisioner_PackageManager_UsesChoco(t *testing.T) {
	runner := newFakeRunner()
	w := newTestWindows(runner)

	if err := w.Provision(context.Background(), domain.ActionConfig{RunnerTemp: t.TempDir()}); err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	script := runner.joined()
	if !strings.Contains(script, "choco install --no-progress -y pritunl-client") {
		t.Errorf("expected choco install, got:\n%s", script)
	}
	if strings.Contains(script, "wireguard") {
		t.Error("did not expect wireguard in ovpn mode")
	}
}

func TestWindowsProvisioner_WireGuardMode_InstallsWG(t *testing.T) {
	runner := newFakeRunner()
	w := newTestWindows(runner)

	if err := w.Provision(context.Background(), domain.ActionConfig{VPNMode: "wg", RunnerTemp: t.TempDir()}); err != nil {
		t.Fatalf("Provision failed: %v", err)
	}
	if !strings.Contains(runner.joined(), "choco install --no-progress -y wireguard") {
		t.Error("expected wireguard choco install in wg mode")
	}
}

func TestWindowsProvisioner_SpecificVersion_UsesInstaller(t *testing.T) {
	runner := newFakeRunner()
	w := newTestWindows(runner)

	cfg := domain.ActionConfig{ClientVersion: "1.3.4696.56", RunnerTemp: t.TempDir()}
	if err := w.Provision(context.Background(), cfg); err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	script := runner.joined()
	if !strings.Contains(script, "releases/download/1.3.4696.56/Pritunl.exe") {
		t.Errorf("expected versioned exe URL, got:\n%s", script)
	}
	if !strings.Contains(script, "pwsh -ExecutionPolicy Bypass -Command Start-Process") {
		t.Error("expected pwsh silent install step")
	}
	if strings.Contains(script, "choco") {
		t.Error("version-specific install must not use choco")
	}
}

func TestWindowsProvisioner_CommandFailures_Propagate(t *testing.T) {
	tests := []struct {
		name    string
		failFor string
		wantMsg string
		version string
	}{
		{name: "choco", failFor: "choco", wantMsg: "failed to install pritunl-client via choco"},
		{name: "curl download", failFor: "curl", wantMsg: "failed to download Pritunl.exe", version: "1.3.4696.56"},
		{name: "pwsh installer", failFor: "pwsh", wantMsg: "failed to run Pritunl installer", version: "1.3.4696.56"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := newFakeRunner()
			runner.errFor[tt.failFor] = errBoom
			w := newTestWindows(runner)

			cfg := domain.ActionConfig{ClientVersion: tt.version, RunnerTemp: t.TempDir()}
			err := w.Provision(context.Background(), cfg)
			if err == nil || !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("expected error containing %q, got: %v", tt.wantMsg, err)
			}
		})
	}
}

func TestWindowsProvisioner_WireGuardFailure_Propagates(t *testing.T) {
	runner := newFakeRunner()
	runner.errFor["choco"] = errBoom
	w := newTestWindows(runner)

	err := w.Provision(context.Background(), domain.ActionConfig{VPNMode: "wg", RunnerTemp: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "failed to install wireguard via choco") {
		t.Fatalf("expected wireguard choco failure, got: %v", err)
	}
}
