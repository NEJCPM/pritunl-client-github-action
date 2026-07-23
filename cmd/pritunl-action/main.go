package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/nathanielvarona/pritunl-client-github-action/pkg/cli"
	"github.com/nathanielvarona/pritunl-client-github-action/pkg/domain"
	"github.com/nathanielvarona/pritunl-client-github-action/pkg/engine"
	"github.com/nathanielvarona/pritunl-client-github-action/pkg/provisioner"
)

func main() {
	cfg := loadConfigFromEnv()

	ctx := context.Background()

	// 1. Provision Pritunl client on host if needed
	prov, err := provisioner.NewProvisioner(cfg.RunnerOS)
	if err != nil {
		fmt.Printf("Error initializing provisioner: %v\n", err)
		os.Exit(1)
	}

	if err := prov.Provision(ctx, cfg); err != nil {
		fmt.Printf("Error provisioning Pritunl client: %v\n", err)
		os.Exit(1)
	}

	// 2. Display installed version
	systemCLI := cli.NewSystemCLI("")
	version, err := systemCLI.Version(ctx)
	if err == nil {
		fmt.Printf("Pritunl Client Installed Version:\n%s\n", version)
	}

	// 3. Connect via VPN Engine
	vpnEngine := engine.NewEngine(systemCLI)
	result, err := vpnEngine.Connect(ctx, cfg)
	if err != nil {
		fmt.Printf("Error executing Pritunl Action Engine: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully configured Pritunl VPN! Connected: %d/%d\n", result.ConnectedCount, result.ExpectedCount)
}

func loadConfigFromEnv() domain.ActionConfig {
	runnerOS := os.Getenv("RUNNER_OS")
	if runnerOS == "" {
		if runtime.GOOS == "darwin" {
			runnerOS = "macOS"
		} else if runtime.GOOS == "windows" {
			runnerOS = "Windows"
		} else {
			runnerOS = "Linux"
		}
	}

	return domain.ActionConfig{
		ProfileFile:                  os.Getenv("PRITUNL_PROFILE_FILE"),
		ProfilePin:                   os.Getenv("PRITUNL_PROFILE_PIN"),
		ProfileServer:                os.Getenv("PRITUNL_PROFILE_SERVER"),
		VPNMode:                      normalizeMode(os.Getenv("PRITUNL_VPN_MODE")),
		ClientVersion:                os.Getenv("PRITUNL_CLIENT_VERSION"),
		StartConnection:              parseBool(os.Getenv("PRITUNL_START_CONNECTION"), true),
		ReadyProfileTimeout:          parseInt(os.Getenv("PRITUNL_READY_PROFILE_TIMEOUT"), 3),
		EstablishedConnectionTimeout: parseInt(os.Getenv("PRITUNL_ESTABLISHED_CONNECTION_TIMEOUT"), 30),
		ConcealedOutputs:             parseBool(os.Getenv("PRITUNL_CONCEALED_OUTPUTS"), true),
		RunnerOS:                     runnerOS,
		RunnerTemp:                   os.Getenv("RUNNER_TEMP"),
		GitHubOutput:                 os.Getenv("GITHUB_OUTPUT"),
		GitHubActions:                os.Getenv("GITHUB_ACTIONS") != "",
	}
}

func normalizeMode(mode string) string {
	m := strings.ToLower(strings.TrimSpace(mode))
	switch m {
	case "wg", "wireguard":
		return "wg"
	case "ovpn", "openvpn":
		return "ovpn"
	default:
		return "ovpn"
	}
}

func parseBool(val string, defaultValue bool) bool {
	if val == "" {
		return defaultValue
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		return defaultValue
	}
	return b
}

func parseInt(val string, defaultValue int) int {
	if val == "" {
		return defaultValue
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return defaultValue
	}
	return i
}
