//go:build e2e

package e2e

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"strings"
)

// RewriteProfileRemoteHost extracts the Pritunl profile tar, replaces the
// remote endpoint of every .ovpn entry with the given host (keeping port and
// protocol), and rebuilds the tar archive. This lets a local test client
// connect through the loopback port-mapping of the test server container.
func RewriteProfileRemoteHost(profileTar []byte, host string) ([]byte, error) {
	reader := tar.NewReader(bytes.NewReader(profileTar))
	var out bytes.Buffer
	writer := tar.NewWriter(&out)

	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading profile tar entry: %w", err)
		}

		content, err := io.ReadAll(io.LimitReader(reader, 4<<20))
		if err != nil {
			return nil, fmt.Errorf("reading profile tar content: %w", err)
		}

		if strings.HasSuffix(header.Name, ".ovpn") {
			content = []byte(rewriteOvpnRemote(string(content), host))
		}

		newHeader := *header
		newHeader.Size = int64(len(content))
		if err := writer.WriteHeader(&newHeader); err != nil {
			return nil, fmt.Errorf("writing profile tar header: %w", err)
		}
		if _, err := writer.Write(content); err != nil {
			return nil, fmt.Errorf("writing profile tar content: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("closing profile tar: %w", err)
	}
	return out.Bytes(), nil
}

// rewriteOvpnRemote replaces the host field of the "remote <host> <port> <proto>" line.
func rewriteOvpnRemote(content string, host string) string {
	var out strings.Builder
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "remote ") {
			fields := strings.Fields(trimmed)
			if len(fields) >= 2 {
				fields[1] = host
				out.WriteString(strings.Join(fields, " "))
				out.WriteString("\n")
				continue
			}
		}
		out.WriteString(line)
		out.WriteString("\n")
	}
	return out.String()
}
