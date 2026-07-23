package domain

// ActionConfig encapsulates all configuration inputs for the Pritunl GitHub Action.
type ActionConfig struct {
	ProfileFile                  string `json:"profile_file"`
	ProfilePin                   string `json:"profile_pin"`
	ProfileServer                string `json:"profile_server"`
	VPNMode                      string `json:"vpn_mode"`
	ClientVersion                string `json:"client_version"`
	StartConnection              bool   `json:"start_connection"`
	ReadyProfileTimeout          int    `json:"ready_profile_timeout"`
	EstablishedConnectionTimeout int    `json:"established_connection_timeout"`
	ConcealedOutputs             bool   `json:"concealed_outputs"`
	RunnerOS                     string `json:"runner_os"`
	RunnerTemp                   string `json:"runner_temp"`
	GitHubOutput                 string `json:"github_output"`
	GitHubActions                bool   `json:"github_actions"`
}

// ProfileServer represents a server entry within a imported Pritunl profile.
type ProfileServer struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ClientAddress string `json:"client_address"`
	Status        string `json:"status"`
}

// ClientIDOutput represents the output structure for a client ID and name pair.
type ClientIDOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ConnectionResult holds the outcome of establishing a VPN connection.
type ConnectionResult struct {
	PrimaryClientID string           `json:"primary_client_id"`
	AllClientIDs    []ClientIDOutput `json:"all_client_ids"`
	ConnectedCount  int              `json:"connected_count"`
	ExpectedCount   int              `json:"expected_count"`
}
