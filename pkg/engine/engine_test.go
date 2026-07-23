package engine

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"testing"

	"github.com/nathanielvarona/pritunl-client-github-action/pkg/cli"
	"github.com/nathanielvarona/pritunl-client-github-action/pkg/domain"
)

func createDummyTarBase64() string {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	body := []byte("dummy profile content")
	hdr := &tar.Header{
		Name: "profile.ovpn",
		Mode: 0600,
		Size: int64(len(body)),
	}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write(body)
	_ = tw.Close()

	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestEngineConnect_Success(t *testing.T) {
	mockCLI := cli.NewMockCLI()
	mockCLI.SetServers([]domain.ProfileServer{
		{ID: "srv-1", Name: "Server A", Status: "disconnected"},
		{ID: "srv-2", Name: "Server B", Status: "disconnected"},
	})

	eng := NewEngine(mockCLI)

	outDir, err := os.MkdirTemp("", "pritunl-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(outDir)

	outFile := outDir + "/github_output"

	cfg := domain.ActionConfig{
		ProfileFile:                  createDummyTarBase64(),
		StartConnection:              true,
		ReadyProfileTimeout:          2,
		EstablishedConnectionTimeout: 2,
		RunnerTemp:                   outDir,
		GitHubOutput:                 outFile,
		GitHubActions:                true,
	}

	result, err := eng.Connect(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if result.PrimaryClientID != "srv-1" {
		t.Errorf("expected primary client ID srv-1, got %s", result.PrimaryClientID)
	}
	if len(result.AllClientIDs) != 1 {
		t.Errorf("expected 1 output server by default, got %d", len(result.AllClientIDs))
	}
	if result.ConnectedCount != 1 {
		t.Errorf("expected 1 connected server, got %d", result.ConnectedCount)
	}

	// Verify output file content
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	if !bytes.Contains(data, []byte("client-id=srv-1")) {
		t.Errorf("output file missing client-id, got: %s", string(data))
	}
}

func TestEngineConnect_AllServersFilter(t *testing.T) {
	mockCLI := cli.NewMockCLI()
	mockCLI.SetServers([]domain.ProfileServer{
		{ID: "srv-1", Name: "Server A", Status: "disconnected"},
		{ID: "srv-2", Name: "Server B", Status: "disconnected"},
	})

	eng := NewEngine(mockCLI)
	outDir, _ := os.MkdirTemp("", "pritunl-test")
	defer os.RemoveAll(outDir)

	cfg := domain.ActionConfig{
		ProfileFile:                  createDummyTarBase64(),
		ProfileServer:                "all-profile-server",
		StartConnection:              true,
		ReadyProfileTimeout:          2,
		EstablishedConnectionTimeout: 2,
		RunnerTemp:                   outDir,
	}

	result, err := eng.Connect(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if len(result.AllClientIDs) != 2 {
		t.Errorf("expected 2 output servers for 'all-profile-server', got %d", len(result.AllClientIDs))
	}
	if result.ConnectedCount != 2 {
		t.Errorf("expected 2 connected servers, got %d", result.ConnectedCount)
	}
}

func TestEngineConnect_InvalidBase64(t *testing.T) {
	mockCLI := cli.NewMockCLI()
	eng := NewEngine(mockCLI)

	cfg := domain.ActionConfig{
		ProfileFile: "invalid-base64-content!!!",
	}

	_, err := eng.Connect(context.Background(), cfg)
	if err == nil {
		t.Error("expected error for invalid base64 profile, got nil")
	}
}
