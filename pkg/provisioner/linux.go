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

type LinuxProvisioner struct{}

func (l *LinuxProvisioner) Provision(ctx context.Context, cfg domain.ActionConfig) error {
	if err := aptUpdate(ctx); err != nil {
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

	if err := ensureMultiarch(ctx); err != nil {
		return fmt.Errorf("failed to configure multiarch: %w", err)
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

		if err := aptUpdate(ctx); err != nil {
			return fmt.Errorf("apt-get update after pritunl repo setup failed: %w", err)
		}
		pkgName := "pritunl-client"
		if runtime.GOARCH == "arm64" {
			pkgName = "pritunl-client:amd64"
		}
		if err := runCmd(ctx, "sudo", "apt-get", "install", "-qq", "-o=Dpkg::Use-Pty=0", "-y", pkgName); err != nil {
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

		if runtime.GOARCH == "arm64" {
			if err := runCmd(ctx, "sudo", "dpkg", "-i", "--force-architecture", installFile); err != nil {
				return fmt.Errorf("failed to install deb package %s: %w", installFile, err)
			}
			if err := runCmd(ctx, "sudo", "apt-get", "install", "-f", "-qq", "-y"); err != nil {
				return fmt.Errorf("failed to fix dependencies after deb install: %w", err)
			}
		} else {
			if err := runCmd(ctx, "sudo", "apt-get", "install", "-qq", "-o=Dpkg::Use-Pty=0", "-y", installFile); err != nil {
				return fmt.Errorf("failed to install deb package %s: %w", installFile, err)
			}
		}
	}

	if _, err := exec.LookPath("pritunl-client"); err != nil {
		return fmt.Errorf("pritunl-client not found in PATH after installation: %w", err)
	}

	return nil
}

func ensureMultiarch(ctx context.Context) error {
	if runtime.GOARCH != "arm64" {
		return nil
	}
	if err := runCmd(ctx, "sudo", "dpkg", "--add-architecture", "amd64"); err != nil {
		return err
	}
	codename, err := getLSBCodename(ctx)
	if err != nil {
		codename = "noble"
	}
	repos := []string{
		fmt.Sprintf("deb [arch=amd64] http://archive.ubuntu.com/ubuntu/ %s main restricted universe multiverse", codename),
		fmt.Sprintf("deb [arch=amd64] http://archive.ubuntu.com/ubuntu/ %s-updates main restricted universe multiverse", codename),
		fmt.Sprintf("deb [arch=amd64] http://archive.ubuntu.com/ubuntu/ %s-backports main restricted universe multiverse", codename),
		fmt.Sprintf("deb [arch=amd64] http://archive.ubuntu.com/ubuntu/ %s-security main restricted universe multiverse", codename),
	}
	if err := writeToFileViaSudo("/etc/apt/sources.list.d/amd64.list", strings.Join(repos, "\n")+"\n"); err != nil {
		return fmt.Errorf("failed to add amd64 apt sources: %w", err)
	}
	_ = aptUpdate(ctx)
	return nil
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

func aptUpdate(ctx context.Context) error {
	err := runCmd(ctx, "sudo", "apt-get", "update", "-qq", "-y")
	if err != nil && runtime.GOARCH == "arm64" {
		return nil
	}
	return err
}

func getTempDir(cfg domain.ActionConfig) string {
	if cfg.RunnerTemp != "" {
		return cfg.RunnerTemp
	}
	return os.TempDir()
}
