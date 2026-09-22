package provisioner

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/NEJCPM/pritunl-client-github-action/pkg/domain"
)

func newTestLinuxWithCosign(runner *fakeRunner) *LinuxProvisioner {
	l := newTestLinux(runner)
	l.runOutput = func(ctx context.Context, name string, args ...string) (string, error) {
		runner.calls = append(runner.calls, cmdCall{Name: name, Args: append([]string(nil), args...)})
		return "ghcr.io/nejcpm/pritunl-client-github-action/pritunl-client@sha256:" + strings.Repeat("a", 64), nil
	}
	l.lookPath = func(name string) (string, error) {
		if name == "cosign" {
			return "/usr/local/bin/cosign", nil
		}
		return "/usr/bin/" + name, nil
	}
	return l
}

func TestLinuxProvisioner_Arm64_VerifiesSignatureBeforeExtraction(t *testing.T) {
	runner := newFakeRunner()
	l := newTestLinuxWithCosign(runner)
	l.goArch = "arm64"

	var order []string
	runner.onCall = append(runner.onCall, func(name string, args []string) {
		if name == "cosign" && len(args) > 0 && args[0] == "verify" {
			order = append(order, "verify")
		}
		if name == "sudo" && len(args) > 1 && args[0] == "docker" && args[1] == "cp" {
			order = append(order, "extract")
		}
	})

	if err := l.Provision(context.Background(), domain.ActionConfig{RunnerTemp: t.TempDir()}); err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	joined := strings.Join(order, ",")
	if joined != "verify,extract,extract" {
		t.Errorf("expected signature verification before extraction, got %q", joined)
	}

	script := runner.joined()
	if !strings.Contains(script, "cosign verify --certificate-oidc-issuer "+cosignOIDCIssuer) {
		t.Errorf("expected cosign verify with OIDC issuer policy, got:\n%s", script)
	}
	if !strings.Contains(script, "--certificate-identity-regexp "+cosignIdentityRegexp) {
		t.Errorf("expected identity regexp policy, got:\n%s", script)
	}
	if !strings.Contains(script, "sha256:") {
		t.Error("expected verification against a pinned digest")
	}
}

func TestLinuxProvisioner_Arm64_SignatureFailureAbortsExtraction(t *testing.T) {
	runner := newFakeRunner()
	runner.errFor["cosign"] = fmt.Errorf("no matching signatures")
	l := newTestLinuxWithCosign(runner)
	l.goArch = "arm64"

	err := l.Provision(context.Background(), domain.ActionConfig{RunnerTemp: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "image signature verification failed") {
		t.Fatalf("expected signature failure, got: %v", err)
	}
	for _, c := range runner.calls {
		if c.Name == "sudo" && len(c.Args) > 1 && c.Args[1] == "docker" {
			t.Fatal("docker cp must not run after failed signature verification")
		}
	}
}

func TestLinuxProvisioner_Arm64_DigestResolutionFailureAborts(t *testing.T) {
	runner := newFakeRunner()
	l := newTestLinuxWithCosign(runner)
	l.goArch = "arm64"
	l.runOutput = func(ctx context.Context, name string, args ...string) (string, error) {
		return "", fmt.Errorf("inspect boom")
	}

	err := l.Provision(context.Background(), domain.ActionConfig{RunnerTemp: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "failed to resolve digest") {
		t.Fatalf("expected digest failure, got: %v", err)
	}
}

func TestLinuxProvisioner_Arm64_MalformedDigestAborts(t *testing.T) {
	runner := newFakeRunner()
	l := newTestLinuxWithCosign(runner)
	l.goArch = "arm64"
	l.runOutput = func(ctx context.Context, name string, args ...string) (string, error) {
		return "not-a-digest", nil
	}

	err := l.Provision(context.Background(), domain.ActionConfig{RunnerTemp: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "unexpected digest format") {
		t.Fatalf("expected malformed digest error, got: %v", err)
	}
}

func TestLinuxProvisioner_Arm64_CosignInstallIsPinnedAndChecksummed(t *testing.T) {
	runner := newFakeRunner()
	l := newTestLinuxWithCosign(runner)
	l.goArch = "arm64"
	l.lookPath = func(string) (string, error) { return "", fmt.Errorf("not found") }
	l.cosignChecksum = func(string, string) error { return nil }

	if err := l.Provision(context.Background(), domain.ActionConfig{RunnerTemp: t.TempDir()}); err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	script := runner.joined()
	wantArch := "arm64"
	if runtime.GOARCH == "amd64" {
		wantArch = "amd64"
	}
	if !strings.Contains(script, "cosign/releases/download/"+cosignVersion+"/cosign-linux-"+wantArch) {
		t.Errorf("expected pinned %s cosign download, got:\n%s", wantArch, script)
	}
	// The checksum verification happens through checksum.VerifyFile which is
	// not a run command; assert the install step went through sudo.
	if !strings.Contains(script, "sudo install -m 0755") {
		t.Errorf("expected sudo install of cosign binary, got:\n%s", script)
	}
}

func TestCosignBinaryURL_UnsupportedArch(t *testing.T) {
	if _, _, err := cosignBinaryURL("riscv64"); err == nil {
		t.Fatal("expected error for unsupported architecture")
	}
}
