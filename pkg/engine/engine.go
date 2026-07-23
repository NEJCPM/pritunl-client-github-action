package engine

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nathanielvarona/pritunl-client-github-action/pkg/cli"
	"github.com/nathanielvarona/pritunl-client-github-action/pkg/domain"
)

// Engine orchestrates profile validation, import, connection start, and status polling.
type Engine struct {
	cli cli.PritunlCLI
}

// NewEngine constructs a new Engine backed by a PritunlCLI adapter seam.
func NewEngine(cliAdapter cli.PritunlCLI) *Engine {
	return &Engine{
		cli: cliAdapter,
	}
}

// Connect performs the entire VPN setup and connection lifecycle.
func (e *Engine) Connect(ctx context.Context, cfg domain.ActionConfig) (*domain.ConnectionResult, error) {
	// 1. Decode base64 profile and validate archive format
	tarPath, err := e.decodeAndValidateProfile(cfg.ProfileFile, cfg.RunnerTemp)
	if err != nil {
		return nil, fmt.Errorf("profile file validation failed: %w", err)
	}
	defer os.Remove(tarPath)

	// 2. Import profile into Pritunl CLI
	if err := e.cli.AddProfile(ctx, tarPath); err != nil {
		return nil, fmt.Errorf("failed to import profile: %w", err)
	}

	// 3. Poll until servers in profile are registered/ready
	targetServers, err := e.waitForReadyServers(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("waiting for ready profile servers failed: %w", err)
	}

	// Sort target servers by name for deterministic primary client-id determination
	sort.Slice(targetServers, func(i, j int) bool {
		return targetServers[i].Name < targetServers[j].Name
	})

	// Determine output structure
	var allOutputs []domain.ClientIDOutput
	for _, s := range targetServers {
		allOutputs = append(allOutputs, domain.ClientIDOutput{
			ID:   s.ID,
			Name: s.Name,
		})
	}

	result := &domain.ConnectionResult{
		PrimaryClientID: targetServers[0].ID,
		AllClientIDs:    allOutputs,
		ExpectedCount:   len(targetServers),
	}

	// 4. Write step outputs if running in GitHub Actions environment
	if cfg.GitHubActions || cfg.GitHubOutput != "" {
		if err := WriteActionOutputs(cfg.GitHubOutput, result.PrimaryClientID, result.AllClientIDs, cfg.ConcealedOutputs); err != nil {
			return nil, fmt.Errorf("failed to write step outputs: %w", err)
		}
	}

	// 5. If auto-start is false, return early after profile setup
	if !cfg.StartConnection {
		return result, nil
	}

	// 6. Start VPN connections for each target server
	for _, s := range targetServers {
		if err := e.cli.StartConnection(ctx, s.ID, cfg.VPNMode, cfg.ProfilePin); err != nil {
			return nil, fmt.Errorf("failed to start connection for server %s (%s): %w", s.Name, s.ID, err)
		}
		time.Sleep(500 * time.Millisecond)
	}

	// 7. Poll until connection is established or timeout is reached
	connectedCount, err := e.waitForEstablishedConnection(ctx, targetServers, cfg.EstablishedConnectionTimeout)
	result.ConnectedCount = connectedCount
	if err != nil {
		if connectedCount > 0 && connectedCount < len(targetServers) {
			// Partial connection tolerance
			fmt.Printf("Warning: connected to %d out of %d target servers within timeout\n", connectedCount, len(targetServers))
		} else {
			return nil, fmt.Errorf("connection establishment failed: %w", err)
		}
	}

	return result, nil
}

func (e *Engine) decodeAndValidateProfile(b64Data string, tempDir string) (string, error) {
	if strings.TrimSpace(b64Data) == "" {
		return "", errors.New("profile-file input is empty")
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64Data))
	if err != nil {
		return "", fmt.Errorf("invalid base64 profile encoding: %w", err)
	}

	// Validate tar header signature
	if !isTarArchive(decoded) {
		return "", errors.New("decoded profile file is not a valid tar archive")
	}

	if tempDir == "" {
		tempDir = os.TempDir()
	}

	outPath := filepath.Join(tempDir, fmt.Sprintf("profile-file-%d.tar", time.Now().UnixNano()))
	if err := os.WriteFile(outPath, decoded, 0600); err != nil {
		return "", fmt.Errorf("failed to write decoded profile tarball: %w", err)
	}

	return outPath, nil
}

func isTarArchive(data []byte) bool {
	tr := tar.NewReader(bytes.NewReader(data))
	_, err := tr.Next()
	return err == nil
}

func (e *Engine) waitForReadyServers(ctx context.Context, cfg domain.ActionConfig) ([]domain.ProfileServer, error) {
	timeout := time.Duration(cfg.ReadyProfileTimeout) * time.Second
	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	deadline := time.Now().Add(timeout)

	for {
		servers, err := e.cli.ListServers(ctx)
		if err == nil && len(servers) > 0 {
			matched := filterServers(servers, cfg.ProfileServer)
			if len(matched) > 0 {
				return matched, nil
			}
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout (%v) reached waiting for profile servers", timeout)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func filterServers(allServers []domain.ProfileServer, serverFilter string) []domain.ProfileServer {
	if serverFilter == "" {
		// Default: return first server sorted by name
		sort.Slice(allServers, func(i, j int) bool {
			return allServers[i].Name < allServers[j].Name
		})
		return []domain.ProfileServer{allServers[0]}
	}

	if serverFilter == "all-profile-server" {
		return allServers
	}

	// Comma-separated matching
	targets := strings.Split(serverFilter, ",")
	var matched []domain.ProfileServer
	for _, rawTarget := range targets {
		target := strings.TrimSpace(rawTarget)
		for _, s := range allServers {
			if strings.Contains(s.Name, target) {
				matched = append(matched, s)
				break
			}
		}
	}

	return matched
}

func (e *Engine) waitForEstablishedConnection(ctx context.Context, targets []domain.ProfileServer, timeoutSec int) (int, error) {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	timeout := time.Duration(timeoutSec) * time.Second
	deadline := time.Now().Add(timeout)

	targetIDs := make(map[string]bool)
	for _, t := range targets {
		targetIDs[t.ID] = true
	}

	for {
		servers, err := e.cli.ListServers(ctx)
		connectedMap := make(map[string]bool)

		if err == nil {
			for _, s := range servers {
				if targetIDs[s.ID] && isValidIPOrCIDR(s.ClientAddress) {
					connectedMap[s.ID] = true
				}
			}
		}

		if len(connectedMap) == len(targetIDs) {
			return len(connectedMap), nil
		}

		if time.Now().After(deadline) {
			return len(connectedMap), fmt.Errorf("timeout (%v) waiting for connection establishment (%d/%d connected)", timeout, len(connectedMap), len(targetIDs))
		}

		select {
		case <-ctx.Done():
			return len(connectedMap), ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
}

func isValidIPOrCIDR(val string) bool {
	if val == "" {
		return false
	}
	if ip := net.ParseIP(val); ip != nil {
		return true
	}
	if _, _, err := net.ParseCIDR(val); err == nil {
		return true
	}
	return false
}
