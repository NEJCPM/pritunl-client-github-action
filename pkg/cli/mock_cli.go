package cli

import (
	"context"
	"errors"
	"sync"

	"github.com/NEJCPM/pritunl-client-github-action/pkg/domain"
)

// MockCLI is an in-memory test adapter implementing PritunlCLI.
type MockCLI struct {
	mu            sync.Mutex
	VersionResult string
	VersionErr    error

	AddProfileErr error
	AddedProfiles []string

	Servers []domain.ProfileServer
	ListErr error

	// FailListAfter makes ListServers return ListErr once the call counter
	// reaches this value (0 disables the behavior; ListErr then applies to
	// every call).
	FailListAfter int
	ListCalls     int

	// SkipAddressFor makes StartConnection NOT assign a ClientAddress for the
	// listed server IDs, simulating servers that never establish a tunnel.
	SkipAddressFor []string

	StartErr     error
	StartedCalls []StartCall
}

type StartCall struct {
	ServerID string
	Mode     string
	Password string
}

func NewMockCLI() *MockCLI {
	return &MockCLI{
		VersionResult: "Pritunl Client v1.3.3884.66",
		Servers:       make([]domain.ProfileServer, 0),
		AddedProfiles: make([]string, 0),
		StartedCalls:  make([]StartCall, 0),
	}
}

func (m *MockCLI) Version(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.VersionErr != nil {
		return "", m.VersionErr
	}
	return m.VersionResult, nil
}

func (m *MockCLI) AddProfile(ctx context.Context, tarPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.AddProfileErr != nil {
		return m.AddProfileErr
	}
	m.AddedProfiles = append(m.AddedProfiles, tarPath)
	return nil
}

func (m *MockCLI) ListServers(ctx context.Context) ([]domain.ProfileServer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ListCalls++
	fail := m.ListErr != nil
	if m.FailListAfter > 0 {
		fail = m.ListCalls >= m.FailListAfter
	}
	if fail && m.ListErr != nil {
		return nil, m.ListErr
	}
	if fail {
		return nil, errors.New("mock list failure")
	}
	return m.Servers, nil
}

func (m *MockCLI) SetServers(servers []domain.ProfileServer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Servers = servers
}

func (m *MockCLI) StartConnection(ctx context.Context, serverID string, mode string, password string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.StartErr != nil {
		return m.StartErr
	}
	m.StartedCalls = append(m.StartedCalls, StartCall{
		ServerID: serverID,
		Mode:     mode,
		Password: password,
	})

	// Simulate status update upon starting
	for i := range m.Servers {
		if m.Servers[i].ID == serverID {
			m.Servers[i].Status = "connected"
			if m.Servers[i].ClientAddress == "" && !containsID(m.SkipAddressFor, serverID) {
				m.Servers[i].ClientAddress = "192.168.233.2/24"
			}
		}
	}

	return nil
}

func containsID(ids []string, id string) bool {
	for _, i := range ids {
		if i == id {
			return true
		}
	}
	return false
}
