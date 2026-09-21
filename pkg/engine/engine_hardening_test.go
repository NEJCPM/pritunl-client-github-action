package engine

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NEJCPM/pritunl-client-github-action/pkg/domain"
)

func TestEngineConnect_RejectsOversizedProfile(t *testing.T) {
	eng, _ := newTestEngine()
	orig := maxProfileBase64Bytes
	maxProfileBase64Bytes = 16
	defer func() { maxProfileBase64Bytes = orig }()

	cfg := baseCfg(t.TempDir(), func(c *domain.ActionConfig) { c.ProfileFile = createDummyTarBase64() })
	_, err := eng.Connect(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum allowed size") {
		t.Fatalf("expected size limit error, got: %v", err)
	}
}

func TestEngineConnect_DedupesRepeatedServerTargets(t *testing.T) {
	eng, mock := newTestEngine()
	mock.SetServers([]domain.ProfileServer{
		{ID: "srv-1", Name: "Server A", Status: "disconnected"},
		{ID: "srv-1", Name: "Server A", Status: "disconnected"},
	})

	result, err := eng.Connect(context.Background(), baseCfg(t.TempDir(), func(c *domain.ActionConfig) {
		c.StartConnection = false
	}))
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if len(result.AllClientIDs) != 1 {
		t.Errorf("expected deduped output, got %d entries", len(result.AllClientIDs))
	}
}

func TestEngineConnect_ProfileTarPathTraversalRejected(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{Name: "../evil.ovpn", Mode: 0600, Size: 4}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write([]byte("data"))
	_ = tw.Close()

	eng, _ := newTestEngine()
	cfg := baseCfg(t.TempDir(), func(c *domain.ActionConfig) { c.ProfileFile = encodeBase64(string(buf.Bytes())) })

	_, err := eng.Connect(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "not a valid tar archive") {
		t.Fatalf("expected traversal tar rejection, got: %v", err)
	}
}

func TestWriteActionOutputs_RejectsControlCharsInID(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "github_output")

	err := WriteActionOutputs(outFile, "srv-1\ninjected=1", nil, true)
	if err == nil || !strings.Contains(err.Error(), "cannot be written to GITHUB_OUTPUT") {
		t.Fatalf("expected output injection rejection, got: %v", err)
	}
	if _, statErr := os.Stat(outFile); statErr == nil {
		t.Error("no output file should be written for invalid ids")
	}
}

func TestWriteActionOutputs_RejectsControlCharsInServerName(t *testing.T) {
	err := WriteActionOutputs("", "srv-1", []domain.ClientIDOutput{{ID: "srv-1", Name: "A\nB"}}, true)
	if err == nil || !strings.Contains(err.Error(), "cannot be written to GITHUB_OUTPUT") {
		t.Fatalf("expected name injection rejection, got: %v", err)
	}
}
