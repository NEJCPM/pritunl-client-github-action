package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/NEJCPM/pritunl-client-github-action/pkg/domain"
)

// maxErrBufferBytes caps how much child-process stderr is captured into
// error messages, so a misbehaving binary cannot flood logs.
const maxErrBufferBytes = 4 << 10 // 4 KiB

// limitedBuffer is an io.Writer that keeps at most max bytes and discards
// the remainder.
type limitedBuffer struct {
	buf bytes.Buffer
	max int
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if l.max > 0 && l.buf.Len()+len(p) > l.max {
		n := l.max - l.buf.Len()
		if n > 0 {
			l.buf.Write(p[:n])
		}
		return len(p), nil
	}
	return l.buf.Write(p)
}

func (l *limitedBuffer) String() string { return l.buf.String() }

// SystemCLI implements PritunlCLI by executing the real pritunl-client binary.
type SystemCLI struct {
	BinaryPath string
}

// NewSystemCLI returns a SystemCLI adapter.
func NewSystemCLI(binaryPath string) *SystemCLI {
	if binaryPath == "" {
		binaryPath = "pritunl-client"
	}
	return &SystemCLI{BinaryPath: binaryPath}
}

func (s *SystemCLI) Version(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, s.BinaryPath, "version")
	var stdout bytes.Buffer
	stderr := &limitedBuffer{max: maxErrBufferBytes}
	cmd.Stdout = &stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to get pritunl-client version: %w (stderr: %s)", err, stderr.String())
	}
	return stdout.String(), nil
}

func (s *SystemCLI) AddProfile(ctx context.Context, tarPath string) error {
	cmd := exec.CommandContext(ctx, s.BinaryPath, "add", tarPath)
	stderr := &limitedBuffer{max: maxErrBufferBytes}
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add profile %s: %w (stderr: %s)", tarPath, err, stderr.String())
	}
	return nil
}

func (s *SystemCLI) ListServers(ctx context.Context) ([]domain.ProfileServer, error) {
	cmd := exec.CommandContext(ctx, s.BinaryPath, "list", "-j")
	var stdout bytes.Buffer
	stderr := &limitedBuffer{max: maxErrBufferBytes}
	cmd.Stdout = &stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to list profile servers: %w (stderr: %s)", err, stderr.String())
	}

	var servers []domain.ProfileServer
	if err := json.Unmarshal(stdout.Bytes(), &servers); err != nil {
		return nil, fmt.Errorf("failed to parse profile server json: %w (raw: %s)", err, stdout.String())
	}
	return servers, nil
}

func (s *SystemCLI) StartConnection(ctx context.Context, serverID string, mode string, password string) error {
	args := []string{"start", serverID}
	if mode != "" {
		args = append(args, "--mode", mode)
	}
	if password != "" {
		args = append(args, "--password", password)
	}

	cmd := exec.CommandContext(ctx, s.BinaryPath, args...)
	stderr := &limitedBuffer{max: maxErrBufferBytes}
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start connection for server %s: %w (stderr: %s)", serverID, err, stderr.String())
	}
	return nil
}
