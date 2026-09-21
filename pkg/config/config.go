// Package config loads the action configuration from environment variables,
// keeping cmd/pritunl-action as a thin wiring-only entrypoint.
package config

import (
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/NEJCPM/pritunl-client-github-action/pkg/domain"
)

// LoadFromEnv builds an ActionConfig from the standard action environment
// variables, applying defaults and normalization to every input.
func LoadFromEnv() domain.ActionConfig {
	runnerOS := os.Getenv("RUNNER_OS")
	if runnerOS == "" {
		switch runtime.GOOS {
		case "darwin":
			runnerOS = "macOS"
		case "windows":
			runnerOS = "Windows"
		default:
			runnerOS = "Linux"
		}
	}

	return domain.ActionConfig{
		ProfileFile:                  os.Getenv("PRITUNL_PROFILE_FILE"),
		ProfilePin:                   os.Getenv("PRITUNL_PROFILE_PIN"),
		ProfileServer:                os.Getenv("PRITUNL_PROFILE_SERVER"),
		VPNMode:                      NormalizeMode(os.Getenv("PRITUNL_VPN_MODE")),
		ClientVersion:                os.Getenv("PRITUNL_CLIENT_VERSION"),
		StartConnection:              ParseBool(os.Getenv("PRITUNL_START_CONNECTION"), true),
		ReadyProfileTimeout:          ParseInt(os.Getenv("PRITUNL_READY_PROFILE_TIMEOUT"), 3),
		EstablishedConnectionTimeout: ParseInt(os.Getenv("PRITUNL_ESTABLISHED_CONNECTION_TIMEOUT"), 30),
		ConcealedOutputs:             ParseBool(os.Getenv("PRITUNL_CONCEALED_OUTPUTS"), true),
		RunnerOS:                     runnerOS,
		RunnerTemp:                   os.Getenv("RUNNER_TEMP"),
		GitHubOutput:                 os.Getenv("GITHUB_OUTPUT"),
		GitHubActions:                os.Getenv("GITHUB_ACTIONS") != "",
	}
}

// NormalizeMode maps user-provided VPN mode aliases to the canonical
// "ovpn"/"wg" values, defaulting to "ovpn".
func NormalizeMode(mode string) string {
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

// ParseBool parses an environment boolean, returning the given default when
// the value is empty or invalid.
func ParseBool(val string, defaultValue bool) bool {
	if val == "" {
		return defaultValue
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		return defaultValue
	}
	return b
}

// ParseInt parses an environment integer, returning the given default when
// the value is empty or invalid.
func ParseInt(val string, defaultValue int) int {
	if val == "" {
		return defaultValue
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return defaultValue
	}
	return i
}
