//go:build e2e

package e2e

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"
)

// PritunlAPIClient drives the Pritunl web API using the temporary default
// administrator session. It never logs credentials, cookies or profile
// contents.
type PritunlAPIClient struct {
	baseURL       string
	httpClient    *http.Client
	csrfToken     string
	authenticated bool
}

func NewPritunlAPIClient(baseURL string) (*PritunlAPIClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	client := &http.Client{
		Transport: tr,
		Jar:       jar,
		Timeout:   60 * time.Second,
	}

	return &PritunlAPIClient{
		baseURL:    baseURL,
		httpClient: client,
	}, nil
}

type authSessionResponse struct {
	Authenticated bool   `json:"authenticated"`
	Default       bool   `json:"default"`
	Error         string `json:"error"`
}

// Authenticate creates an administrator session and fetches the CSRF token.
// The server healthcheck can report healthy slightly before the web layer is
// fully serving, so transient connection errors and 5xx responses are retried.
func (c *PritunlAPIClient) Authenticate(username, password string) error {
	payload := map[string]string{
		"username": username,
		"password": password,
	}
	body, _ := json.Marshal(payload)

	const attempts = 12
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		resp, err := c.doRaw(http.MethodPost, "/auth/session", bytes.NewReader(body), "application/json")
		if err != nil {
			lastErr = fmt.Errorf("auth session request failed: %w", err)
		} else {
			respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
			closeErr := resp.Body.Close()
			if readErr != nil {
				lastErr = readErr
			} else if closeErr != nil {
				lastErr = closeErr
			} else if resp.StatusCode == http.StatusOK {
				var parsed authSessionResponse
				if err := json.Unmarshal(respBody, &parsed); err != nil {
					lastErr = fmt.Errorf("auth session parse failed: %w", err)
				} else if !parsed.Authenticated {
					// Wrong credentials are a hard failure: do not retry.
					return fmt.Errorf("authentication rejected by server")
				} else {
					c.authenticated = true
					return c.refreshCSRF()
				}
			} else if resp.StatusCode >= 500 {
				lastErr = fmt.Errorf("authentication server error status %d: %s", resp.StatusCode, truncate(string(respBody), 200))
			} else {
				return fmt.Errorf("authentication failed status %d: %s", resp.StatusCode, truncate(string(respBody), 200))
			}
		}

		if attempt < attempts {
			time.Sleep(5 * time.Second)
		}
	}
	return fmt.Errorf("%v (after %d attempts)", lastErr, attempts)
}

// refreshCSRF reads the CSRF token from /state.
func (c *PritunlAPIClient) refreshCSRF() error {
	resp, err := c.doRaw(http.MethodGet, "/state", nil, "")
	if err != nil {
		return fmt.Errorf("state request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("state request failed status %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var state struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.Unmarshal(respBody, &state); err != nil {
		return fmt.Errorf("state parse failed: %w", err)
	}
	if state.CSRFToken == "" {
		return fmt.Errorf("empty csrf token")
	}
	c.csrfToken = state.CSRFToken
	return nil
}

// doRaw performs an HTTP request with the session cookies attached.
func (c *PritunlAPIClient) doRaw(method, path string, body io.Reader, contentType string) (*http.Response, error) {
	req, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return c.httpClient.Do(req)
}

// do performs an authenticated API request. On the first 401 it refreshes the
// CSRF token and retries once.
func (c *PritunlAPIClient) do(method, path string, body []byte) (*http.Response, error) {
	if !c.authenticated {
		return nil, fmt.Errorf("client not authenticated")
	}

	for attempt := 0; attempt < 2; attempt++ {
		var reader io.Reader
		if body != nil {
			reader = bytes.NewReader(body)
		}

		req, err := http.NewRequest(method, c.baseURL+path, reader)
		if err != nil {
			return nil, err
		}
		req.Header.Set("PR-Validated", "true")
		req.Header.Set("Csrf-Token", c.csrfToken)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			resp.Body.Close()
			if err := c.refreshCSRF(); err != nil {
				return nil, fmt.Errorf("csrf refresh failed: %w", err)
			}
			continue
		}
		return resp, nil
	}
	return nil, fmt.Errorf("unreachable")
}

// doJSON performs an authenticated request and decodes the JSON response.
func (c *PritunlAPIClient) doJSON(method, path string, body []byte, out interface{}) error {
	resp, err := c.do(method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s failed status %d: %s", method, path, resp.StatusCode, truncate(string(respBody), 300))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(respBody, out)
}

// CreateOrganization creates the e2e organization, or reuses an existing one.
func (c *PritunlAPIClient) CreateOrganization(name string) (string, error) {
	var orgs []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := c.doJSON(http.MethodGet, "/organization", nil, &orgs); err != nil {
		return "", err
	}
	for _, o := range orgs {
		if o.Name == name {
			return o.ID, nil
		}
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := c.doJSON(http.MethodPost, "/organization", mustJSON(map[string]string{"name": name}), &created); err != nil {
		return "", err
	}
	return created.ID, nil
}

// CreateUser creates a client user with the given PIN.
func (c *PritunlAPIClient) CreateUser(orgID, name, pin string) (string, error) {
	var created []struct {
		ID string `json:"id"`
	}
	if err := c.doJSON(http.MethodPost, fmt.Sprintf("/user/%s", orgID), mustJSON(map[string]string{
		"name":  name,
		"email": name + "@e2e.invalid",
		"pin":   pin,
	}), &created); err != nil {
		return "", err
	}
	if len(created) == 0 {
		return "", fmt.Errorf("user creation returned no users")
	}
	return created[0].ID, nil
}

// CreateServer creates an OpenVPN server on the standard port.
func (c *PritunlAPIClient) CreateServer(name, network string) (string, error) {
	var servers []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := c.doJSON(http.MethodGet, "/server", nil, &servers); err != nil {
		return "", err
	}
	for _, s := range servers {
		if s.Name == name {
			return s.ID, nil
		}
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := c.doJSON(http.MethodPost, "/server", mustJSON(map[string]interface{}{
		"name":        name,
		"port":        1194,
		"protocol":    "udp",
		"network":     network,
		"cipher":      "aes256",
		"hash":        "sha256",
		"ipv6":        false,
		"dns_servers": []string{},
	}), &created); err != nil {
		return "", err
	}
	return created.ID, nil
}

// AttachOrganization links an organization to a server. If the server is
// online the operation is rejected, so the server is stopped first.
func (c *PritunlAPIClient) AttachOrganization(serverID, orgID string) error {
	err := c.doJSON(http.MethodPut, fmt.Sprintf("/server/%s/organization/%s", serverID, orgID), nil, nil)
	if err == nil {
		return nil
	}
	if !strings.Contains(err.Error(), "server_not_offline") {
		return err
	}
	if err := c.StopServer(serverID); err != nil {
		return fmt.Errorf("failed to stop server before attach: %w", err)
	}
	if err := c.waitForStatus(serverID, "offline", 30*time.Second); err != nil {
		return err
	}
	return c.doJSON(http.MethodPut, fmt.Sprintf("/server/%s/organization/%s", serverID, orgID), nil, nil)
}

// StopServer issues the stop operation.
func (c *PritunlAPIClient) StopServer(serverID string) error {
	return c.doJSON(http.MethodPut, fmt.Sprintf("/server/%s/operation/stop", serverID), nil, nil)
}

// waitForStatus polls until the server reports the wanted status.
func (c *PritunlAPIClient) waitForStatus(serverID, want string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	lastStatus := "unknown"
	for time.Now().Before(deadline) {
		status, err := c.GetServerStatus(serverID)
		if err == nil {
			lastStatus = status
			if status == want {
				return nil
			}
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("server did not reach %q within %v (last status %q)", want, timeout, lastStatus)
}

// StartServer issues the start operation. The first call may only generate
// DH parameters; callers must poll GetServerStatus afterwards.
func (c *PritunlAPIClient) StartServer(serverID string) error {
	return c.doJSON(http.MethodPut, fmt.Sprintf("/server/%s/operation/start", serverID), nil, nil)
}

// GetServerStatus returns the current server status string.
func (c *PritunlAPIClient) GetServerStatus(serverID string) (string, error) {
	var server struct {
		Status string `json:"status"`
	}
	if err := c.doJSON(http.MethodGet, fmt.Sprintf("/server/%s", serverID), nil, &server); err != nil {
		return "", err
	}
	return server.Status, nil
}

// WaitForServerOnline polls until the server reports online. A "pending"
// state that persists beyond pendingGrace is recovered with a stop/start
// cycle; a flaky runner listener can leave the first start half-done.
func (c *PritunlAPIClient) WaitForServerOnline(serverID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	lastStatus := "unknown"
	var pendingSince time.Time
	for time.Now().Before(deadline) {
		status, err := c.GetServerStatus(serverID)
		if err == nil {
			lastStatus = status
			switch status {
			case "online":
				return nil
			case "offline":
				// Start may have only generated DH parameters on the first
				// call; issue the operation again before waiting further.
				_ = c.StartServer(serverID)
				pendingSince = time.Time{}
			case "pending":
				if pendingSince.IsZero() {
					pendingSince = time.Now()
				} else if time.Since(pendingSince) > 45*time.Second {
					_ = c.StopServer(serverID)
					if waitErr := c.waitForStatus(serverID, "offline", 30*time.Second); waitErr == nil {
						_ = c.StartServer(serverID)
					}
					pendingSince = time.Now()
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("server did not become online within %v (last status %q)", timeout, lastStatus)
}

// GetUserKeyTar downloads the user profile tar archive.
func (c *PritunlAPIClient) GetUserKeyTar(orgID, userID string) ([]byte, error) {
	resp, err := c.do(http.MethodGet, fmt.Sprintf("/data/%s/%s.tar", orgID, userID), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("profile download failed status %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	return body, nil
}

func mustJSON(v interface{}) []byte {
	data, _ := json.Marshal(v)
	return data
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...[truncated]"
}
