//go:build !windows

package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFakeClient creates an executable shell script that emulates the
// pritunl-client CLI for unit tests.
func writeFakeClient(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "pritunl-client")
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write fake client: %v", err)
	}
	return path
}

func TestNewSystemCLI_DefaultBinary(t *testing.T) {
	if got := NewSystemCLI("").BinaryPath; got != "pritunl-client" {
		t.Errorf("default binary path = %q, want pritunl-client", got)
	}
	if got := NewSystemCLI("/custom/path").BinaryPath; got != "/custom/path" {
		t.Errorf("custom binary path = %q", got)
	}
}

func TestSystemCLI_Version_Success(t *testing.T) {
	bin := writeFakeClient(t, `echo "Pritunl Client v1.3.4696.56"`)
	s := NewSystemCLI(bin)

	version, err := s.Version(context.Background())
	if err != nil {
		t.Fatalf("Version failed: %v", err)
	}
	if !strings.Contains(version, "v1.3.4696.56") {
		t.Errorf("unexpected version output: %q", version)
	}
}

func TestSystemCLI_Version_Error(t *testing.T) {
	bin := writeFakeClient(t, `echo "boom" >&2; exit 1`)
	s := NewSystemCLI(bin)

	_, err := s.Version(context.Background())
	if err == nil || !strings.Contains(err.Error(), "failed to get pritunl-client version") {
		t.Fatalf("expected version error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Error("expected stderr to be included in error")
	}
}

func TestSystemCLI_AddProfile_Success(t *testing.T) {
	bin := writeFakeClient(t, `exit 0`)
	s := NewSystemCLI(bin)

	if err := s.AddProfile(context.Background(), "/tmp/profile.tar"); err != nil {
		t.Fatalf("AddProfile failed: %v", err)
	}
}

func TestSystemCLI_AddProfile_Error(t *testing.T) {
	bin := writeFakeClient(t, `echo "invalid tar" >&2; exit 1`)
	s := NewSystemCLI(bin)

	err := s.AddProfile(context.Background(), "/tmp/bad.tar")
	if err == nil || !strings.Contains(err.Error(), "failed to add profile /tmp/bad.tar") {
		t.Fatalf("expected add profile error, got: %v", err)
	}
}

func TestSystemCLI_ListServers_Success(t *testing.T) {
	bin := writeFakeClient(t, `echo '[{"id":"srv-1","name":"A","status":"connected","client_address":"10.0.0.2"}]'`)
	s := NewSystemCLI(bin)

	servers, err := s.ListServers(context.Background())
	if err != nil {
		t.Fatalf("ListServers failed: %v", err)
	}
	if len(servers) != 1 || servers[0].ID != "srv-1" {
		t.Fatalf("unexpected servers: %+v", servers)
	}
}

func TestSystemCLI_ListServers_CommandError(t *testing.T) {
	bin := writeFakeClient(t, `echo "service down" >&2; exit 1`)
	s := NewSystemCLI(bin)

	_, err := s.ListServers(context.Background())
	if err == nil || !strings.Contains(err.Error(), "failed to list profile servers") {
		t.Fatalf("expected list error, got: %v", err)
	}
}

func TestSystemCLI_ListServers_InvalidJSON(t *testing.T) {
	bin := writeFakeClient(t, `echo 'not-json'`)
	s := NewSystemCLI(bin)

	_, err := s.ListServers(context.Background())
	if err == nil || !strings.Contains(err.Error(), "failed to parse profile server json") {
		t.Fatalf("expected json parse error, got: %v", err)
	}
}

func TestSystemCLI_StartConnection_Arguments(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	bin := writeFakeClient(t, `printf '%s\n' "$@" > `+argsFile+`; exit 0`)

	s := NewSystemCLI(bin)
	if err := s.StartConnection(context.Background(), "srv-1", "wg", "secret"); err != nil {
		t.Fatalf("StartConnection failed: %v", err)
	}

	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read captured args: %v", err)
	}
	got := strings.Fields(string(data))
	want := []string{"start", "srv-1", "--mode", "wg", "--password", "secret"}
	if len(got) != len(want) {
		t.Fatalf("captured args %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("arg[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSystemCLI_StartConnection_OptionalArgsOmitted(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	bin := writeFakeClient(t, `printf '%s\n' "$@" > `+argsFile+`; exit 0`)

	s := NewSystemCLI(bin)
	if err := s.StartConnection(context.Background(), "srv-1", "", ""); err != nil {
		t.Fatalf("StartConnection failed: %v", err)
	}

	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read captured args: %v", err)
	}
	if strings.TrimSpace(string(data)) != "start\nsrv-1" && string(data) != "start\nsrv-1\n" {
		t.Errorf("unexpected args: %q", string(data))
	}
}

func TestSystemCLI_StartConnection_Error(t *testing.T) {
	bin := writeFakeClient(t, `echo "start failed" >&2; exit 1`)
	s := NewSystemCLI(bin)

	err := s.StartConnection(context.Background(), "srv-9", "ovpn", "pin")
	if err == nil || !strings.Contains(err.Error(), "failed to start connection for server srv-9") {
		t.Fatalf("expected start error, got: %v", err)
	}
}
