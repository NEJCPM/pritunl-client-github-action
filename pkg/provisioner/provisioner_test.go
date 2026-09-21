package provisioner

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/NEJCPM/pritunl-client-github-action/pkg/domain"
)

// cmdCall records one executed command for assertions.
type cmdCall struct {
	Name string
	Args []string
}

// fakeRunner is an in-memory commandRunner seam.
type fakeRunner struct {
	calls   []cmdCall
	errFor  map[string]error
	always  error
	ignored map[string]bool // commands whose failures are tolerated by prod code
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{errFor: map[string]error{}, ignored: map[string]bool{}}
}

func (f *fakeRunner) run(_ context.Context, name string, args ...string) error {
	f.calls = append(f.calls, cmdCall{Name: name, Args: append([]string(nil), args...)})
	if f.always != nil {
		return f.always
	}
	// Match failures by binary name or, for wrappers like sudo, by subcommand.
	if err, ok := f.errFor[name]; ok {
		return err
	}
	if len(args) > 0 {
		if err, ok := f.errFor[args[0]]; ok {
			return err
		}
	}
	return nil
}

func (f *fakeRunner) all(name string) []cmdCall {
	var out []cmdCall
	for _, c := range f.calls {
		if c.Name == name {
			out = append(out, c)
		}
	}
	return out
}

func (f *fakeRunner) joined() string {
	var b strings.Builder
	for _, c := range f.calls {
		b.WriteString(c.Name)
		b.WriteString(" ")
		b.WriteString(strings.Join(c.Args, " "))
		b.WriteString("\n")
	}
	return b.String()
}

func fakeLinux(l *LinuxProvisioner, runner *fakeRunner) {
	l.run = runner.run
	l.showKeys = func(context.Context, string) (string, error) {
		return "fpr:::::::::" + expectedPritunlKeyFingerprint, nil
	}
	l.lsbCodename = func(context.Context) (string, error) { return "jammy", nil }
	l.lookPath = func(string) (string, error) { return "/usr/bin/pritunl-client", nil }
	l.writeFile = func(context.Context, string, string) error { return nil }
	l.goArch = "amd64"
}

func newTestLinux(runner *fakeRunner) *LinuxProvisioner {
	l := NewLinuxProvisioner()
	fakeLinux(l, runner)
	return l
}

func TestNewProvisioner_Factory(t *testing.T) {
	tests := []struct {
		os      string
		want    string
		wantErr bool
	}{
		{os: "Linux", want: "*provisioner.LinuxProvisioner"},
		{os: "macOS", want: "*provisioner.MacOSProvisioner"},
		{os: "Darwin", want: "*provisioner.MacOSProvisioner"},
		{os: "Windows", want: "*provisioner.WindowsProvisioner"},
		{os: "Solaris", wantErr: true},
		{os: "", wantErr: true},
	}

	for _, tt := range tests {
		p, err := NewProvisioner(tt.os)
		if tt.wantErr {
			if err == nil {
				t.Errorf("NewProvisioner(%q): expected error, got nil", tt.os)
			}
			continue
		}
		if err != nil {
			t.Fatalf("NewProvisioner(%q): unexpected error: %v", tt.os, err)
		}
		if got := fmt.Sprintf("%T", p); got != tt.want {
			t.Errorf("NewProvisioner(%q) = %s, want %s", tt.os, got, tt.want)
		}
	}
}

func TestLinuxProvisioner_PackageManagerInstall_UsesSignedByKeyring(t *testing.T) {
	runner := newFakeRunner()
	l := newTestLinux(runner)

	cfg := domain.ActionConfig{VPNMode: "ovpn", RunnerTemp: t.TempDir()}
	if err := l.Provision(context.Background(), cfg); err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	script := runner.joined()

	if !strings.Contains(script, "gnupg") {
		t.Error("expected gnupg install command")
	}
	if !strings.Contains(script, pritunlGPGKeyURL) {
		t.Errorf("expected key download from %s", pritunlGPGKeyURL)
	}
	if !strings.Contains(script, "gpg --dearmor --yes -o /usr/share/keyrings/pritunl.gpg") {
		t.Error("expected dearmor into /usr/share/keyrings/pritunl.gpg")
	}
	if strings.Contains(script, "keyserver") {
		t.Error("must not use keyserver-based key retrieval anymore")
	}
	if !strings.Contains(script, "apt-get update") || !strings.Contains(script, "apt-get install") {
		t.Error("expected apt-get update and install commands")
	}
	if !strings.Contains(script, "openvpn-systemd-resolved") {
		t.Error("expected ovpn dependency install")
	}

	updates := 0
	for _, c := range runner.all("sudo") {
		if len(c.Args) > 0 && c.Args[0] == "apt-get" && c.Args[1] == "update" {
			updates++
		}
	}
	if updates != 2 {
		t.Errorf("expected 2 apt-get update calls, got %d", updates)
	}
}

func TestLinuxProvisioner_RepoLine_ContainsSignedByAndCodename(t *testing.T) {
	runner := newFakeRunner()
	l := newTestLinux(runner)

	var writtenPath, writtenContent string
	l.writeFile = func(_ context.Context, path string, content string) error {
		writtenPath, writtenContent = path, content
		return nil
	}

	cfg := domain.ActionConfig{RunnerTemp: t.TempDir()}
	if err := l.Provision(context.Background(), cfg); err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	if writtenPath != "/etc/apt/sources.list.d/pritunl.list" {
		t.Errorf("unexpected repo file path: %s", writtenPath)
	}
	if !strings.Contains(writtenContent, "signed-by=/usr/share/keyrings/pritunl.gpg") {
		t.Errorf("repo line missing signed-by: %s", writtenContent)
	}
	if !strings.Contains(writtenContent, "jammy main") {
		t.Errorf("repo line missing detected codename: %s", writtenContent)
	}
}

func TestLinuxProvisioner_LSBFailure_FallsBackToNoble(t *testing.T) {
	runner := newFakeRunner()
	l := newTestLinux(runner)
	l.lsbCodename = func(context.Context) (string, error) { return "", fmt.Errorf("not found") }

	var writtenContent string
	l.writeFile = func(_ context.Context, _ string, content string) error {
		writtenContent = content
		return nil
	}

	if err := l.Provision(context.Background(), domain.ActionConfig{RunnerTemp: t.TempDir()}); err != nil {
		t.Fatalf("Provision failed: %v", err)
	}
	if !strings.Contains(writtenContent, "noble main") {
		t.Errorf("expected noble fallback in repo line, got: %s", writtenContent)
	}
}

func TestLinuxProvisioner_WireGuardMode_InstallsWGTools(t *testing.T) {
	runner := newFakeRunner()
	l := newTestLinux(runner)

	if err := l.Provision(context.Background(), domain.ActionConfig{VPNMode: "wg", RunnerTemp: t.TempDir()}); err != nil {
		t.Fatalf("Provision failed: %v", err)
	}
	if !strings.Contains(runner.joined(), "wireguard-tools") {
		t.Error("expected wireguard-tools dependency")
	}
	if strings.Contains(runner.joined(), "openvpn-systemd-resolved") {
		t.Error("did not expect ovpn dependency in wg mode")
	}
}

func TestLinuxProvisioner_SpecificVersion_DownloadsDeb(t *testing.T) {
	runner := newFakeRunner()
	l := newTestLinux(runner)

	cfg := domain.ActionConfig{ClientVersion: "1.3.4696.56", RunnerTemp: t.TempDir()}
	if err := l.Provision(context.Background(), cfg); err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	script := runner.joined()
	if !strings.Contains(script, "pritunl-client_1.3.4696.56-0ubuntu1.jammy_amd64.deb") {
		t.Errorf("expected versioned deb URL, got:\n%s", script)
	}
	if strings.Contains(script, "gnupg") || strings.Contains(script, "pritunl.gpg") {
		t.Error("version-specific install must not configure apt repository")
	}
}

func TestLinuxProvisioner_CommandFailures_Propagate(t *testing.T) {
	tests := []struct {
		name    string
		failFor string
		wantMsg string
	}{
		{name: "initial apt update", failFor: "sudo", wantMsg: "apt-get update failed"},
		{name: "key download", failFor: "curl", wantMsg: "failed to download pritunl repository GPG key"},
		{name: "gpg dearmor", failFor: "gpg", wantMsg: "failed to install pritunl repository GPG keyring"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := newFakeRunner()
			runner.errFor[tt.failFor] = fmt.Errorf("boom")
			l := newTestLinux(runner)

			err := l.Provision(context.Background(), domain.ActionConfig{RunnerTemp: t.TempDir()})
			if err == nil {
				t.Fatalf("expected error for %s failure", tt.failFor)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantMsg)
			}
		})
	}
}

func TestLinuxProvisioner_WriteRepoFailure_Propagates(t *testing.T) {
	runner := newFakeRunner()
	l := newTestLinux(runner)
	l.writeFile = func(context.Context, string, string) error { return fmt.Errorf("disk full") }

	err := l.Provision(context.Background(), domain.ActionConfig{RunnerTemp: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "failed to configure pritunl apt repository") {
		t.Fatalf("expected repo write error, got: %v", err)
	}
}

func TestLinuxProvisioner_LookPathFailure_Propagates(t *testing.T) {
	runner := newFakeRunner()
	l := newTestLinux(runner)
	l.lookPath = func(string) (string, error) { return "", fmt.Errorf("not in PATH") }

	err := l.Provision(context.Background(), domain.ActionConfig{RunnerTemp: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "not found in PATH") {
		t.Fatalf("expected lookPath error, got: %v", err)
	}
}

func TestLinuxProvisioner_Arm64_UsesDockerFlow(t *testing.T) {
	runner := newFakeRunner()
	l := newTestLinux(runner)
	l.goArch = "arm64"

	var unitContent string
	l.writeFile = func(_ context.Context, path string, content string) error {
		if strings.Contains(path, "systemd") {
			unitContent = content
		}
		return nil
	}

	cfg := domain.ActionConfig{RunnerTemp: t.TempDir()}
	if err := l.Provision(context.Background(), cfg); err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	script := runner.joined()
	if !strings.Contains(script, "docker pull ghcr.io/nejcpm/pritunl-client-github-action/pritunl-client:1.3.4696.56") {
		t.Errorf("expected default version docker pull, got:\n%s", script)
	}
	if !strings.Contains(script, "--entrypoint /pritunl-client") {
		t.Error("expected docker create with entrypoint")
	}
	if !strings.Contains(script, "docker cp pritunl-extract-") {
		t.Error("expected binary extraction")
	}
	if !strings.Contains(script, "systemctl enable --now pritunl-client") {
		t.Error("expected systemd service enable")
	}
	if !strings.Contains(unitContent, "ExecStart=/usr/bin/pritunl-client-service") {
		t.Errorf("unexpected systemd unit: %s", unitContent)
	}
}

func TestLinuxProvisioner_Arm64_SpecificVersion(t *testing.T) {
	runner := newFakeRunner()
	l := newTestLinux(runner)
	l.goArch = "arm64"

	cfg := domain.ActionConfig{ClientVersion: "1.3.4000.0", RunnerTemp: t.TempDir()}
	if err := l.Provision(context.Background(), cfg); err != nil {
		t.Fatalf("Provision failed: %v", err)
	}
	if !strings.Contains(runner.joined(), "pritunl-client:1.3.4000.0") {
		t.Error("expected versioned image tag")
	}
}

func TestLinuxProvisioner_Arm64_DockerPullFailure(t *testing.T) {
	runner := newFakeRunner()
	runner.errFor["docker"] = fmt.Errorf("registry down")
	l := newTestLinux(runner)
	l.goArch = "arm64"

	err := l.Provision(context.Background(), domain.ActionConfig{RunnerTemp: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "failed to pull") {
		t.Fatalf("expected pull failure, got: %v", err)
	}
}

func TestLinuxProvisioner_Arm64_SystemctlFailure(t *testing.T) {
	runner := newFakeRunner()
	l := newTestLinux(runner)
	l.goArch = "arm64"
	runner.errFor["systemctl"] = fmt.Errorf("no systemd")

	err := l.Provision(context.Background(), domain.ActionConfig{RunnerTemp: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "no systemd") {
		t.Fatalf("expected systemctl failure propagation, got: %v", err)
	}
}

func TestResolveArm64Version(t *testing.T) {
	tests := []struct {
		clientVersion string
		want          string
	}{
		{clientVersion: "", want: defaultPritunlVersion},
		{clientVersion: "from-package-manager", want: defaultPritunlVersion},
		{clientVersion: "1.3.4696.56", want: "1.3.4696.56"},
	}

	for _, tt := range tests {
		if got := resolveArm64Version(domain.ActionConfig{ClientVersion: tt.clientVersion}); got != tt.want {
			t.Errorf("resolveArm64Version(%q) = %q, want %q", tt.clientVersion, got, tt.want)
		}
	}
}

func TestGetTempDir(t *testing.T) {
	if got := getTempDir(domain.ActionConfig{RunnerTemp: "/custom/tmp"}); got != "/custom/tmp" {
		t.Errorf("getTempDir with RunnerTemp = %q", got)
	}
	if got := getTempDir(domain.ActionConfig{}); got == "" {
		t.Error("getTempDir without RunnerTemp must fall back to os.TempDir")
	}
}
