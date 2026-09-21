package config

import (
	"testing"

	"github.com/NEJCPM/pritunl-client-github-action/pkg/domain"
)

func TestLoadFromEnv_AllValuesSet(t *testing.T) {
	t.Setenv("PRITUNL_PROFILE_FILE", "profile-b64")
	t.Setenv("PRITUNL_PROFILE_PIN", "1234")
	t.Setenv("PRITUNL_PROFILE_SERVER", "srv-1, srv-2")
	t.Setenv("PRITUNL_VPN_MODE", "wireguard")
	t.Setenv("PRITUNL_CLIENT_VERSION", "1.3.4696.56")
	t.Setenv("PRITUNL_START_CONNECTION", "false")
	t.Setenv("PRITUNL_READY_PROFILE_TIMEOUT", "10")
	t.Setenv("PRITUNL_ESTABLISHED_CONNECTION_TIMEOUT", "60")
	t.Setenv("PRITUNL_CONCEALED_OUTPUTS", "false")
	t.Setenv("RUNNER_OS", "Linux")
	t.Setenv("RUNNER_TEMP", "/runner/tmp")
	t.Setenv("GITHUB_OUTPUT", "/runner/out")
	t.Setenv("GITHUB_ACTIONS", "true")

	cfg := LoadFromEnv()

	want := domain.ActionConfig{
		ProfileFile:                  "profile-b64",
		ProfilePin:                   "1234",
		ProfileServer:                "srv-1, srv-2",
		VPNMode:                      "wg",
		ClientVersion:                "1.3.4696.56",
		StartConnection:              false,
		ReadyProfileTimeout:          10,
		EstablishedConnectionTimeout: 60,
		ConcealedOutputs:             false,
		RunnerOS:                     "Linux",
		RunnerTemp:                   "/runner/tmp",
		GitHubOutput:                 "/runner/out",
		GitHubActions:                true,
	}
	if cfg != want {
		t.Errorf("cfg = %+v, want %+v", cfg, want)
	}
}

func TestLoadFromEnv_Defaults(t *testing.T) {
	cfg := LoadFromEnv()

	if cfg.VPNMode != "ovpn" {
		t.Errorf("default VPNMode = %q, want ovpn", cfg.VPNMode)
	}
	if !cfg.StartConnection {
		t.Error("default StartConnection should be true")
	}
	if cfg.ReadyProfileTimeout != 3 {
		t.Errorf("default ReadyProfileTimeout = %d, want 3", cfg.ReadyProfileTimeout)
	}
	if cfg.EstablishedConnectionTimeout != 30 {
		t.Errorf("default EstablishedConnectionTimeout = %d, want 30", cfg.EstablishedConnectionTimeout)
	}
	if !cfg.ConcealedOutputs {
		t.Error("default ConcealedOutputs should be true")
	}
	if cfg.GitHubActions {
		t.Error("GitHubActions should be false without GITHUB_ACTIONS")
	}
	switch cfg.RunnerOS {
	case "Linux", "macOS", "Windows":
		// valid per host OS
	default:
		t.Errorf("unexpected fallback RunnerOS: %q", cfg.RunnerOS)
	}
}

func TestLoadFromEnv_InvalidValuesFallBackToDefaults(t *testing.T) {
	t.Setenv("PRITUNL_START_CONNECTION", "not-a-bool")
	t.Setenv("PRITUNL_CONCEALED_OUTPUTS", "maybe")
	t.Setenv("PRITUNL_READY_PROFILE_TIMEOUT", "abc")
	t.Setenv("PRITUNL_ESTABLISHED_CONNECTION_TIMEOUT", "-")

	cfg := LoadFromEnv()

	if !cfg.StartConnection {
		t.Error("invalid bool should fall back to default true")
	}
	if !cfg.ConcealedOutputs {
		t.Error("invalid bool should fall back to default true")
	}
	if cfg.ReadyProfileTimeout != 3 {
		t.Errorf("invalid int should fall back to 3, got %d", cfg.ReadyProfileTimeout)
	}
	if cfg.EstablishedConnectionTimeout != 30 {
		t.Errorf("invalid int should fall back to 30, got %d", cfg.EstablishedConnectionTimeout)
	}
}

func TestNormalizeMode(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "", want: "ovpn"},
		{in: "ovpn", want: "ovpn"},
		{in: "OpenVPN", want: "ovpn"},
		{in: " openvpn ", want: "ovpn"},
		{in: "wg", want: "wg"},
		{in: "WG", want: "wg"},
		{in: "wireguard", want: "wg"},
		{in: "WireGuard ", want: "wg"},
		{in: "unknown", want: "ovpn"},
	}

	for _, tt := range tests {
		if got := NormalizeMode(tt.in); got != tt.want {
			t.Errorf("NormalizeMode(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseBool(t *testing.T) {
	tests := []struct {
		in   string
		def  bool
		want bool
	}{
		{in: "", def: true, want: true},
		{in: "", def: false, want: false},
		{in: "true", def: false, want: true},
		{in: "false", def: true, want: false},
		{in: "1", def: false, want: true},
		{in: "0", def: true, want: false},
		{in: "garbage", def: true, want: true},
	}

	for _, tt := range tests {
		if got := ParseBool(tt.in, tt.def); got != tt.want {
			t.Errorf("ParseBool(%q, %v) = %v, want %v", tt.in, tt.def, got, tt.want)
		}
	}
}

func TestParseInt(t *testing.T) {
	tests := []struct {
		in   string
		def  int
		want int
	}{
		{in: "", def: 7, want: 7},
		{in: "42", def: 7, want: 42},
		{in: "-5", def: 7, want: -5},
		{in: "abc", def: 7, want: 7},
	}

	for _, tt := range tests {
		if got := ParseInt(tt.in, tt.def); got != tt.want {
			t.Errorf("ParseInt(%q, %d) = %d, want %d", tt.in, tt.def, got, tt.want)
		}
	}
}

func TestClampInt(t *testing.T) {
	tests := []struct {
		val, def, max, want int
	}{
		{val: 0, def: 3, max: 300, want: 3},
		{val: -5, def: 3, max: 300, want: 3},
		{val: 10, def: 3, max: 300, want: 10},
		{val: 1000, def: 3, max: 300, want: 300},
	}

	for _, tt := range tests {
		if got := ClampInt(tt.val, tt.def, tt.max); got != tt.want {
			t.Errorf("ClampInt(%d, %d, %d) = %d, want %d", tt.val, tt.def, tt.max, got, tt.want)
		}
	}
}

func TestLoadFromEnv_ClampsHugeTimeouts(t *testing.T) {
	t.Setenv("PRITUNL_READY_PROFILE_TIMEOUT", "99999")
	t.Setenv("PRITUNL_ESTABLISHED_CONNECTION_TIMEOUT", "99999")

	cfg := LoadFromEnv()
	if cfg.ReadyProfileTimeout > 300 {
		t.Errorf("ReadyProfileTimeout not clamped: %d", cfg.ReadyProfileTimeout)
	}
	if cfg.EstablishedConnectionTimeout > 900 {
		t.Errorf("EstablishedConnectionTimeout not clamped: %d", cfg.EstablishedConnectionTimeout)
	}
}
