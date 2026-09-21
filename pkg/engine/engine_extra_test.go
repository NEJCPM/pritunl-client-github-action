package engine

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NEJCPM/pritunl-client-github-action/pkg/cli"
	"github.com/NEJCPM/pritunl-client-github-action/pkg/domain"
)

func newTestEngine() (*Engine, *cli.MockCLI) {
	mock := cli.NewMockCLI()
	return NewEngine(mock), mock
}

func baseCfg(tempDir string, overrides ...func(*domain.ActionConfig)) domain.ActionConfig {
	cfg := domain.ActionConfig{
		ProfileFile:                  createDummyTarBase64(),
		StartConnection:              true,
		ReadyProfileTimeout:          2,
		EstablishedConnectionTimeout: 2,
		RunnerTemp:                   tempDir,
	}
	for _, o := range overrides {
		o(&cfg)
	}
	return cfg
}

func TestEngineConnect_ProfileValidationError(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		wantMsg string
	}{
		{name: "empty profile", profile: "", wantMsg: "profile file validation failed"},
		{name: "invalid base64", profile: "!!!not-base64!!!", wantMsg: "invalid base64 profile encoding"},
		{name: "not a tar archive", profile: encodeBase64("plain text"), wantMsg: "not a valid tar archive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng, _ := newTestEngine()
			cfg := baseCfg(t.TempDir(), func(c *domain.ActionConfig) { c.ProfileFile = tt.profile })

			_, err := eng.Connect(context.Background(), cfg)
			if err == nil || !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("expected error containing %q, got: %v", tt.wantMsg, err)
			}
		})
	}
}

func encodeBase64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func createDummyTarBase64Decoded() string {
	data, _ := base64.StdEncoding.DecodeString(createDummyTarBase64())
	return string(data)
}

func TestEngineConnect_AddProfileFailure(t *testing.T) {
	eng, mock := newTestEngine()
	mock.AddProfileErr = fmt.Errorf("import boom")

	_, err := eng.Connect(context.Background(), baseCfg(t.TempDir()))
	if err == nil || !strings.Contains(err.Error(), "failed to import profile") {
		t.Fatalf("expected import error, got: %v", err)
	}
}

func TestEngineConnect_NoServersReadyTimeout(t *testing.T) {
	eng, _ := newTestEngine()

	start := time.Now()
	_, err := eng.Connect(context.Background(), baseCfg(t.TempDir(), func(c *domain.ActionConfig) {
		c.ReadyProfileTimeout = 1
	}))
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("expected ready timeout error, got: %v", err)
	}
	if time.Since(start) < 900*time.Millisecond {
		t.Error("expected waitForReadyServers to poll until deadline")
	}
}

func TestEngineConnect_StartConnectionFalse_SkipsConnection(t *testing.T) {
	eng, mock := newTestEngine()
	mock.SetServers([]domain.ProfileServer{
		{ID: "srv-1", Name: "Server A", Status: "disconnected"},
	})

	result, err := eng.Connect(context.Background(), baseCfg(t.TempDir(), func(c *domain.ActionConfig) {
		c.StartConnection = false
	}))
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if result.ConnectedCount != 0 {
		t.Errorf("ConnectedCount = %d, want 0 when start-connection is false", result.ConnectedCount)
	}
	if len(mock.StartedCalls) != 0 {
		t.Errorf("expected no StartConnection calls, got %d", len(mock.StartedCalls))
	}
}

func TestEngineConnect_StartConnectionFails(t *testing.T) {
	eng, mock := newTestEngine()
	mock.SetServers([]domain.ProfileServer{{ID: "srv-1", Name: "Server A", Status: "disconnected"}})
	mock.StartErr = fmt.Errorf("start boom")

	_, err := eng.Connect(context.Background(), baseCfg(t.TempDir()))
	if err == nil || !strings.Contains(err.Error(), "failed to start connection for server Server A (srv-1)") {
		t.Fatalf("expected start failure error, got: %v", err)
	}
}

func TestEngineConnect_EstablishedTimeout_PartialTolerance(t *testing.T) {
	eng, mock := newTestEngine()
	mock.SetServers([]domain.ProfileServer{
		{ID: "srv-1", Name: "Server A", Status: "disconnected"},
		{ID: "srv-2", Name: "Server B", Status: "disconnected"},
	})
	// Server B never obtains a tunnel address.
	mock.SkipAddressFor = []string{"srv-2"}

	result, err := eng.Connect(context.Background(), baseCfg(t.TempDir(), func(c *domain.ActionConfig) {
		c.ProfileServer = "all-profile-server"
		c.EstablishedConnectionTimeout = 1
	}))
	if err != nil {
		t.Fatalf("partial connection should be tolerated, got: %v", err)
	}
	if result.ConnectedCount != 1 {
		t.Errorf("ConnectedCount = %d, want 1", result.ConnectedCount)
	}
}

func TestEngineConnect_EstablishedTimeout_TotalFailure(t *testing.T) {
	eng, mock := newTestEngine()
	mock.SetServers([]domain.ProfileServer{{ID: "srv-1", Name: "Server A", Status: "disconnected"}})
	// After start, make status polling fail so no address is ever observed.
	mock.FailListAfter = 2
	mock.ListErr = errors.New("service unavailable")

	cfg := baseCfg(t.TempDir(), func(c *domain.ActionConfig) {
		c.EstablishedConnectionTimeout = 1
	})
	_, err := eng.Connect(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "connection establishment failed") {
		t.Fatalf("expected establishment failure, got: %v", err)
	}
}

func TestEngineConnect_ContextCancelled(t *testing.T) {
	eng, _ := newTestEngine()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := eng.Connect(ctx, baseCfg(t.TempDir(), func(c *domain.ActionConfig) {
		c.ReadyProfileTimeout = 5
	}))
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestFilterServers(t *testing.T) {
	servers := []domain.ProfileServer{
		{ID: "srv-2", Name: "Bravo"},
		{ID: "srv-1", Name: "Alpha"},
		{ID: "srv-3", Name: "Charlie"},
	}

	tests := []struct {
		name     string
		filter   string
		wantIDs  []string
		wantLen  int
		sortBack bool // whether input order mutation matters for the assertion
	}{
		{name: "default returns first sorted", filter: "", wantIDs: []string{"srv-1"}},
		{name: "all-profile-server returns everything", filter: "all-profile-server", wantLen: 3},
		{name: "single name match", filter: "Alpha", wantIDs: []string{"srv-1"}},
		{name: "comma separated matches", filter: "Bravo, Charlie", wantIDs: []string{"srv-2", "srv-3"}},
		{name: "no match returns empty", filter: "Zulu", wantLen: 0},
		{name: "empty target among list ignored", filter: ", Alpha", wantIDs: []string{"srv-1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := append([]domain.ProfileServer(nil), servers...)
			got := filterServers(input, tt.filter)

			if tt.wantLen != 0 && len(got) != tt.wantLen {
				t.Fatalf("len = %d, want %d", len(got), tt.wantLen)
			}
			if tt.wantIDs != nil {
				gotIDs := make([]string, len(got))
				for i, s := range got {
					gotIDs[i] = s.ID
				}
				if fmt.Sprint(gotIDs) != fmt.Sprint(tt.wantIDs) {
					t.Errorf("ids = %v, want %v", gotIDs, tt.wantIDs)
				}
			}
		})
	}
}

func TestIsValidIPOrCIDR(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{in: "", want: false},
		{in: "192.168.1.1", want: true},
		{in: "192.168.1.1/24", want: true},
		{in: "::1", want: true},
		{in: "2001:db8::/32", want: true},
		{in: "not-an-ip", want: false},
		{in: "999.999.999.999", want: false},
		{in: "192.168.1.1/", want: false},
	}

	for _, tt := range tests {
		if got := isValidIPOrCIDR(tt.in); got != tt.want {
			t.Errorf("isValidIPOrCIDR(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestDecodeAndValidateProfile(t *testing.T) {
	eng, _ := newTestEngine()
	temp := t.TempDir()

	path, err := eng.decodeAndValidateProfile(createDummyTarBase64(), temp)
	if err != nil {
		t.Fatalf("decodeAndValidateProfile failed: %v", err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected written tarball: %v", err)
	}
	if !isTarArchive(data) {
		t.Error("written file is not a valid tar archive")
	}
	if filepath.Dir(path) != temp {
		t.Errorf("file written to %s, want inside %s", path, temp)
	}
}

func TestDecodeAndValidateProfile_EmptyTempDir(t *testing.T) {
	eng, _ := newTestEngine()

	path, err := eng.decodeAndValidateProfile(createDummyTarBase64(), "")
	if err != nil {
		t.Fatalf("decodeAndValidateProfile failed: %v", err)
	}
	defer os.Remove(path)

	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file to exist: %v", err)
	}
}

func TestIsTarArchive(t *testing.T) {
	if !isTarArchive([]byte(createDummyTarBase64Decoded())) {
		t.Error("expected tar content to validate")
	}
	if isTarArchive([]byte("definitely not tar")) {
		t.Error("expected non-tar content to fail validation")
	}
}

func TestWriteActionOutputs_WritesFile(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "github_output")

	err := WriteActionOutputs(outFile, "srv-1", []domain.ClientIDOutput{{ID: "srv-1", Name: "A"}}, true)
	if err != nil {
		t.Fatalf("WriteActionOutputs failed: %v", err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("output file missing: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "client-id=srv-1\n") {
		t.Errorf("missing client-id in output: %q", content)
	}
	if !strings.Contains(content, `"id":"srv-1"`) {
		t.Errorf("missing client-ids JSON in output: %q", content)
	}
}

func TestWriteActionOutputs_EmptyPathStillLogs(t *testing.T) {
	if err := WriteActionOutputs("", "srv-1", nil, true); err != nil {
		t.Fatalf("empty output path should not fail: %v", err)
	}
}

func TestWriteActionOutputs_InvalidOutputPath(t *testing.T) {
	err := WriteActionOutputs(filepath.Join(t.TempDir(), "missing-dir", "out"), "srv-1", nil, true)
	if err == nil || !strings.Contains(err.Error(), "failed to open GITHUB_OUTPUT") {
		t.Fatalf("expected open failure, got: %v", err)
	}
}

func TestEngineConnect_ListErrorWaitsAndTimesOut(t *testing.T) {
	eng, mock := newTestEngine()
	mock.ListErr = errors.New("service unavailable")

	_, err := eng.Connect(context.Background(), baseCfg(t.TempDir(), func(c *domain.ActionConfig) {
		c.ReadyProfileTimeout = 1
	}))
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("expected timeout despite list errors, got: %v", err)
	}
}
