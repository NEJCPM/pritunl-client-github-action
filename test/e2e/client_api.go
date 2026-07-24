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
	"time"
)

type PritunlAPIClient struct {
	baseURL    string
	httpClient *http.Client
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
		Timeout:   15 * time.Second,
	}

	return &PritunlAPIClient{
		baseURL:    baseURL,
		httpClient: client,
	}, nil
}

func (c *PritunlAPIClient) Authenticate(username, password string) error {
	payload := map[string]string{
		"username": username,
		"password": password,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", c.baseURL+"/auth/session", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("authentication failed status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
