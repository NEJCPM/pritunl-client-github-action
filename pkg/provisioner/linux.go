package provisioner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/NEJCPM/pritunl-client-github-action/pkg/checksum"
	"github.com/NEJCPM/pritunl-client-github-action/pkg/domain"
)

const defaultPritunlVersion = "1.3.4696.56"
const pritunlImageRepo = "ghcr.io/nejcpm/pritunl-client-github-action/pritunl-client"
const pritunlGPGKeyURL = "https://raw.githubusercontent.com/pritunl/pgp/master/pritunl_repo_pub.asc"

// expectedPritunlKeyFingerprint is the official Pritunl repository signing
// key fingerprint. The downloaded key file must contain it.
const expectedPritunlKeyFingerprint = "7568D9BB55FF9E5287D586017AE645C0CF8E292A"

// validCodenamePattern restricts apt distro codenames to safe values so a
// misbehaving lsb_release cannot corrupt the sources list.
var validCodenamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)

// LinuxProvisioner installs the Pritunl client on Debian-based runners.
// Command execution, privileged file writes, LSB detection and PATH lookup are
// injectable seams so unit tests never run real system commands.
type LinuxProvisioner struct {
	run            commandRunner
	showKeys       func(ctx context.Context, keyFile string) (string, error)
	lsbCodename    func(ctx context.Context) (string, error)
	lookPath       func(string) (string, error)
	writeFile      func(ctx context.Context, filePath string, content string) error
	runOutput      func(ctx context.Context, name string, args ...string) (string, error)
	verify         artifactVerifier
	cosignChecksum func(path string, expected string) error
	goArch         string
}

// NewLinuxProvisioner returns a LinuxProvisioner wired to the real system.
func NewLinuxProvisioner() *LinuxProvisioner {
	return &LinuxProvisioner{
		run:            runCmd,
		showKeys:       showKeyFingerprints,
		lsbCodename:    getLSBCodename,
		lookPath:       exec.LookPath,
		writeFile:      writeToFileViaSudo,
		runOutput:      runCmdOutput,
		verify:         newArtifactVerifier(),
		cosignChecksum: checksum.VerifyFile,
		goArch:         runtime.GOARCH,
	}
}

func (l *LinuxProvisioner) Provision(ctx context.Context, cfg domain.ActionConfig) error {
	if err := l.run(ctx, "sudo", "apt-get", "update", "-qq", "-y"); err != nil {
		return fmt.Errorf("apt-get update failed: %w", err)
	}

	deps := []string{"net-tools", "iptables", "openvpn", "resolvconf"}
	if cfg.VPNMode == "ovpn" {
		deps = append(deps, "openvpn-systemd-resolved")
	} else if cfg.VPNMode == "wg" {
		deps = append(deps, "wireguard-tools")
	}

	aptArgs := append([]string{"apt-get", "install", "-qq", "-o=Dpkg::Use-Pty=0", "-y"}, deps...)
	if err := l.run(ctx, "sudo", aptArgs...); err != nil {
		return fmt.Errorf("failed to install VPN dependencies: %w", err)
	}

	if l.goArch == "arm64" {
		return l.provisionFromDocker(ctx, cfg)
	}

	if cfg.ClientVersion == "" || cfg.ClientVersion == "from-package-manager" {
		if err := l.run(ctx, "sudo", "apt-get", "install", "-qq", "-o=Dpkg::Use-Pty=0", "-y", "gnupg"); err != nil {
			return fmt.Errorf("failed to install gnupg: %w", err)
		}

		distroCodename, err := l.lsbCodename(ctx)
		if err != nil {
			distroCodename = "noble"
		}
		if !validCodenamePattern.MatchString(distroCodename) {
			return fmt.Errorf("detected invalid distro codename %q; refusing to configure apt repository", distroCodename)
		}

		keyFile := fmt.Sprintf("%s/pritunl_repo_pub.asc", getTempDir(cfg))
		if err := l.run(ctx, "curl", "-fsSL", pritunlGPGKeyURL, "-o", keyFile); err != nil {
			return fmt.Errorf("failed to download pritunl repository GPG key: %w", err)
		}
		defer os.Remove(keyFile)

		keyOut, keyErr := l.showKeys(ctx, keyFile)
		if keyErr != nil {
			return fmt.Errorf("failed to inspect downloaded pritunl GPG key: %w", keyErr)
		}
		if !strings.Contains(keyOut, expectedPritunlKeyFingerprint) {
			return fmt.Errorf("downloaded pritunl GPG key does not match expected fingerprint %s; aborting repository setup", expectedPritunlKeyFingerprint)
		}

		if err := l.run(ctx, "sudo", "gpg", "--dearmor", "--yes", "-o", "/usr/share/keyrings/pritunl.gpg", keyFile); err != nil {
			return fmt.Errorf("failed to install pritunl repository GPG keyring: %w", err)
		}

		repoLine := fmt.Sprintf("deb [ signed-by=/usr/share/keyrings/pritunl.gpg ] https://repo.pritunl.com/stable/apt %s main", distroCodename)
		if err := l.writeFile(ctx, "/etc/apt/sources.list.d/pritunl.list", repoLine); err != nil {
			return fmt.Errorf("failed to configure pritunl apt repository: %w", err)
		}

		if err := l.run(ctx, "sudo", "apt-get", "update", "-qq", "-y"); err != nil {
			return fmt.Errorf("apt-get update after pritunl repo setup failed: %w", err)
		}
		if err := l.run(ctx, "sudo", "apt-get", "install", "-qq", "-o=Dpkg::Use-Pty=0", "-y", "pritunl-client"); err != nil {
			return fmt.Errorf("failed to install pritunl-client package: %w", err)
		}
	} else {
		distroCodename, err := l.lsbCodename(ctx)
		if err != nil {
			distroCodename = "noble"
		}
		if !validCodenamePattern.MatchString(distroCodename) {
			return fmt.Errorf("detected invalid distro codename %q; refusing to download distro-specific deb package", distroCodename)
		}
		artifactKey := fmt.Sprintf("deb-%s-amd64", distroCodename)
		debURL := fmt.Sprintf("https://github.com/pritunl/pritunl-client-electron/releases/download/%s/pritunl-client_%s-0ubuntu1.%s_amd64.deb", cfg.ClientVersion, cfg.ClientVersion, distroCodename)
		installFile := fmt.Sprintf("%s/pritunl-client.deb", getTempDir(cfg))

		if err := l.run(ctx, "curl", "-fsSL", debURL, "-o", installFile); err != nil {
			return fmt.Errorf("failed to download deb package from %s: %w", debURL, err)
		}
		defer os.Remove(installFile)

		if err := l.verify(cfg, artifactKey, installFile); err != nil {
			return err
		}

		if err := l.run(ctx, "sudo", "apt-get", "install", "-qq", "-o=Dpkg::Use-Pty=0", "-y", installFile); err != nil {
			return fmt.Errorf("failed to install deb package %s: %w", installFile, err)
		}
	}

	if _, err := l.lookPath("pritunl-client"); err != nil {
		return fmt.Errorf("pritunl-client not found in PATH after installation: %w", err)
	}

	return nil
}

func resolveArm64Version(cfg domain.ActionConfig) string {
	if cfg.ClientVersion == "" || cfg.ClientVersion == "from-package-manager" {
		return defaultPritunlVersion
	}
	return cfg.ClientVersion
}

func (l *LinuxProvisioner) provisionFromDocker(ctx context.Context, cfg domain.ActionConfig) error {
	version := resolveArm64Version(cfg)
	image := pritunlImageRepo + ":" + version
	// Unique container name prevents collisions between concurrent jobs on
	// self-hosted runners.
	containerName := fmt.Sprintf("pritunl-extract-%d", os.Getpid())

	if err := l.run(ctx, "docker", "pull", image); err != nil {
		return fmt.Errorf("failed to pull %s: %w", image, err)
	}

	digest, err := l.resolveImageDigest(ctx, image)
	if err != nil {
		return err
	}
	if err := l.verifyImageSignature(ctx, cfg, digest); err != nil {
		return err
	}

	_ = l.run(ctx, "docker", "rm", "-f", containerName)

	if err := l.run(ctx, "docker", "create", "--name", containerName, "--entrypoint", "/pritunl-client", image); err != nil {
		return err
	}
	defer l.run(ctx, "docker", "rm", "-f", containerName)

	if err := l.run(ctx, "sudo", "docker", "cp", containerName+":/pritunl-client", "/usr/bin/pritunl-client"); err != nil {
		return err
	}
	if err := l.run(ctx, "sudo", "docker", "cp", containerName+":/pritunl-client-service", "/usr/bin/pritunl-client-service"); err != nil {
		return err
	}
	if err := l.run(ctx, "sudo", "chmod", "+x", "/usr/bin/pritunl-client", "/usr/bin/pritunl-client-service"); err != nil {
		return err
	}

	return l.startPritunlService(ctx)
}

func (l *LinuxProvisioner) startPritunlService(ctx context.Context) error {
	unit := `[Unit]
Description=Pritunl Client Daemon

[Service]
ExecStart=/usr/bin/pritunl-client-service
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`
	if err := l.writeFile(ctx, "/etc/systemd/system/pritunl-client.service", unit); err != nil {
		return err
	}
	if err := l.run(ctx, "sudo", "systemctl", "daemon-reload"); err != nil {
		return err
	}
	return l.run(ctx, "sudo", "systemctl", "enable", "--now", "pritunl-client")
}

func runCmdOutput(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	return string(out), err
}

func getLSBCodename(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "lsb_release", "-cs")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// showKeyFingerprints runs gpg --show-keys on the downloaded key file and
// returns its colonized output for fingerprint validation.
func showKeyFingerprints(ctx context.Context, keyFile string) (string, error) {
	cmd := exec.CommandContext(ctx, "gpg", "--show-keys", "--with-colons", keyFile)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func writeToFileViaSudo(ctx context.Context, filePath string, content string) error {
	cmd := exec.CommandContext(ctx, "sudo", "tee", filePath)
	cmd.Stdin = strings.NewReader(content)
	return cmd.Run()
}

func runCmd(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func getTempDir(cfg domain.ActionConfig) string {
	if cfg.RunnerTemp != "" {
		return cfg.RunnerTemp
	}
	return os.TempDir()
}

// createTempFile creates a uniquely named empty file under dir. On failure it
// returns an empty path so callers treat the whole operation as failed.
func createTempFile(dir string, pattern string) (string, error) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return f.Name(), nil
}
