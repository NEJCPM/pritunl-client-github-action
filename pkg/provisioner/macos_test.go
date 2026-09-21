package provisioner

import (
	"context"
	"strings"
	"testing"

	"github.com/NEJCPM/pritunl-client-github-action/pkg/domain"
)

func newTestMacOS(runner *fakeRunner) *MacOSProvisioner {
	m := NewMacOSProvisioner()
	m.run = runner.run
	return m
}

func TestMacOSProvisioner_PackageManager_UsesBrewCask(t *testing.T) {
	runner := newFakeRunner()
	m := newTestMacOS(runner)
	t.Setenv("HOME", t.TempDir())

	if err := m.Provision(context.Background(), domain.ActionConfig{RunnerTemp: t.TempDir()}); err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	script := runner.joined()
	if !strings.Contains(script, "brew install -q --cask pritunl") {
		t.Errorf("expected brew cask install, got:\n%s", script)
	}
	if strings.Contains(script, "wireguard-tools") {
		t.Error("did not expect wireguard-tools in ovpn mode")
	}
}

func TestMacOSProvisioner_WireGuardMode_InstallsWGTools(t *testing.T) {
	runner := newFakeRunner()
	m := newTestMacOS(runner)
	t.Setenv("HOME", t.TempDir())

	if err := m.Provision(context.Background(), domain.ActionConfig{VPNMode: "wg", RunnerTemp: t.TempDir()}); err != nil {
		t.Fatalf("Provision failed: %v", err)
	}
	if !strings.Contains(runner.joined(), "brew install -q wireguard-tools") {
		t.Error("expected wireguard-tools install in wg mode")
	}
}

func TestMacOSProvisioner_SpecificVersion_UsesInstaller(t *testing.T) {
	runner := newFakeRunner()
	m := newTestMacOS(runner)
	temp := t.TempDir()
	t.Setenv("HOME", temp)

	cfg := domain.ActionConfig{ClientVersion: "1.3.4696.56", RunnerTemp: temp}
	if err := m.Provision(context.Background(), cfg); err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	script := runner.joined()
	if !strings.Contains(script, "releases/download/1.3.4696.56/Pritunl.pkg.zip") {
		t.Errorf("expected versioned pkg zip URL, got:\n%s", script)
	}
	if !strings.Contains(script, "unzip -qq -o") {
		t.Error("expected unzip step")
	}
	if !strings.Contains(script, "installer -pkg") {
		t.Error("expected installer step")
	}
	if strings.Contains(script, "brew") {
		t.Error("version-specific install must not use brew")
	}
}

func TestMacOSProvisioner_CommandFailures_Propagate(t *testing.T) {
	tests := []struct {
		name    string
		failFor string
		wantMsg string
		version string
	}{
		{name: "brew cask", failFor: "brew", wantMsg: "failed to install pritunl cask via brew"},
		{name: "curl download", failFor: "curl", wantMsg: "failed to download macOS pkg zip", version: "1.3.4696.56"},
		{name: "unzip", failFor: "unzip", wantMsg: "failed to unzip", version: "1.3.4696.56"},
		{name: "installer", failFor: "installer", wantMsg: "failed to install macOS pkg", version: "1.3.4696.56"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := newFakeRunner()
			runner.errFor[tt.failFor] = errBoom
			m := newTestMacOS(runner)
			t.Setenv("HOME", t.TempDir())

			cfg := domain.ActionConfig{ClientVersion: tt.version, RunnerTemp: t.TempDir()}
			err := m.Provision(context.Background(), cfg)
			if err == nil || !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("expected error containing %q, got: %v", tt.wantMsg, err)
			}
		})
	}
}
