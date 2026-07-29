package provisioner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/nathanielvarona/pritunl-client-github-action/pkg/domain"
)

const defaultPritunlVersion = "1.3.4696.56"
const pritunlImageRepo = "ghcr.io/nejcpm/pritunl-client-github-action/pritunl-client"

type LinuxProvisioner struct{}

func (l *LinuxProvisioner) Provision(ctx context.Context, cfg domain.ActionConfig) error {
	if err := runCmd(ctx, "sudo", "apt-get", "update", "-qq", "-y"); err != nil {
		return fmt.Errorf("apt-get update failed: %w", err)
	}

	deps := []string{"net-tools", "iptables", "openvpn", "resolvconf"}
	if cfg.VPNMode == "ovpn" {
		deps = append(deps, "openvpn-systemd-resolved")
	} else if cfg.VPNMode == "wg" {
		deps = append(deps, "wireguard-tools")
	}

	aptArgs := append([]string{"apt-get", "install", "-qq", "-o=Dpkg::Use-Pty=0", "-y"}, deps...)
	if err := runCmd(ctx, "sudo", aptArgs...); err != nil {
		return fmt.Errorf("failed to install VPN dependencies: %w", err)
	}

	if runtime.GOARCH == "arm64" {
		return provisionFromDocker(ctx, cfg)
	}

	if cfg.ClientVersion == "" || cfg.ClientVersion == "from-package-manager" {
		gpgKey := os.Getenv("PRITUNL_LINUX_RUNNER_GPG_KEY")
		if gpgKey == "" {
			gpgKey = "7568D9BB55FF9E5287D586017AE645C0CF8E292A"
		}

		distroCodename, err := getLSBCodename(ctx)
		if err != nil {
			distroCodename = "noble"
		}

		repoLine := fmt.Sprintf("deb https://repo.pritunl.com/stable/apt %s main", distroCodename)
		if err := writeToFileViaSudo("/etc/apt/sources.list.d/pritunl.list", repoLine); err != nil {
			return fmt.Errorf("failed to configure pritunl apt repository: %w", err)
		}

		_ = runCmd(ctx, "gpg", "--keyserver", "hkp://keyserver.ubuntu.com", "--recv-keys", gpgKey)
		gpgExportCmd := exec.CommandContext(ctx, "gpg", "--armor", "--export", gpgKey)
		gpgOut, err := gpgExportCmd.Output()
		if err == nil {
			_ = writeToFileViaSudo("/etc/apt/trusted.gpg.d/pritunl.asc", string(gpgOut))
		}

		if err := runCmd(ctx, "sudo", "apt-get", "update", "-qq", "-y"); err != nil {
			return fmt.Errorf("apt-get update after pritunl repo setup failed: %w", err)
		}
		if err := runCmd(ctx, "sudo", "apt-get", "install", "-qq", "-o=Dpkg::Use-Pty=0", "-y", "pritunl-client"); err != nil {
			return fmt.Errorf("failed to install pritunl-client package: %w", err)
		}
	} else {
		distroCodename, err := getLSBCodename(ctx)
		if err != nil {
			distroCodename = "noble"
		}
		debURL := fmt.Sprintf("https://github.com/pritunl/pritunl-client-electron/releases/download/%s/pritunl-client_%s-0ubuntu1.%s_amd64.deb", cfg.ClientVersion, cfg.ClientVersion, distroCodename)
		installFile := fmt.Sprintf("%s/pritunl-client.deb", getTempDir(cfg))

		if err := runCmd(ctx, "curl", "-sSL", debURL, "-o", installFile); err != nil {
			return fmt.Errorf("failed to download deb package from %s: %w", debURL, err)
		}
		defer os.Remove(installFile)

		if err := runCmd(ctx, "sudo", "apt-get", "install", "-qq", "-o=Dpkg::Use-Pty=0", "-y", installFile); err != nil {
			return fmt.Errorf("failed to install deb package %s: %w", installFile, err)
		}
	}

	if _, err := exec.LookPath("pritunl-client"); err != nil {
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

func provisionFromDocker(ctx context.Context, cfg domain.ActionConfig) error {
	version := resolveArm64Version(cfg)
	image := pritunlImageRepo + ":" + version
	containerName := "pritunl-extract"

	if err := runCmd(ctx, "docker", "pull", image); err != nil {
		return fmt.Errorf("failed to pull %s: %w", image, err)
	}

	_ = runCmd(ctx, "docker", "rm", "-f", containerName)

	if err := runCmd(ctx, "docker", "create", "--name", containerName, "--entrypoint", "/pritunl-client", image); err != nil {
		return err
	}
	defer runCmd(ctx, "docker", "rm", "-f", containerName)

	if err := runCmd(ctx, "sudo", "docker", "cp", containerName+":/pritunl-client", "/usr/bin/pritunl-client"); err != nil {
		return err
	}
	if err := runCmd(ctx, "sudo", "docker", "cp", containerName+":/pritunl-client-service", "/usr/bin/pritunl-client-service"); err != nil {
		return err
	}
	if err := runCmd(ctx, "sudo", "chmod", "+x", "/usr/bin/pritunl-client", "/usr/bin/pritunl-client-service"); err != nil {
		return err
	}

	return startPritunlService(ctx)
}

func startPritunlService(ctx context.Context) error {
	unit := `[Unit]
Description=Pritunl Client Daemon

[Service]
ExecStart=/usr/bin/pritunl-client-service
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`
	if err := writeToFileViaSudo("/etc/systemd/system/pritunl-client.service", unit); err != nil {
		return err
	}
	if err := runCmd(ctx, "sudo", "systemctl", "daemon-reload"); err != nil {
		return err
	}
	return runCmd(ctx, "sudo", "systemctl", "enable", "--now", "pritunl-client")
}

func getLSBCodename(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "lsb_release", "-cs")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func writeToFileViaSudo(filePath string, content string) error {
	cmd := exec.Command("sudo", "tee", filePath)
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
