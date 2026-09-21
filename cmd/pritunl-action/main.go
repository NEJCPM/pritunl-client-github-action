package main

import (
	"context"
	"fmt"
	"os"

	"github.com/NEJCPM/pritunl-client-github-action/pkg/cli"
	"github.com/NEJCPM/pritunl-client-github-action/pkg/config"
	"github.com/NEJCPM/pritunl-client-github-action/pkg/engine"
	"github.com/NEJCPM/pritunl-client-github-action/pkg/provisioner"
)

func main() {
	cfg := config.LoadFromEnv()

	ctx := context.Background()

	// 1. Provision Pritunl client on host if needed
	prov, err := provisioner.NewProvisioner(cfg.RunnerOS)
	if err != nil {
		fmt.Printf("Error initializing provisioner: %v\n", err)
		os.Exit(1)
	}

	if err := prov.Provision(ctx, cfg); err != nil {
		fmt.Printf("Error provisioning Pritunl client: %v\n", err)
		os.Exit(1)
	}

	// 2. Display installed version
	systemCLI := cli.NewSystemCLI("")
	version, err := systemCLI.Version(ctx)
	if err == nil {
		fmt.Printf("Pritunl Client Installed Version:\n%s\n", version)
	}

	// 3. Connect via VPN Engine
	vpnEngine := engine.NewEngine(systemCLI)
	result, err := vpnEngine.Connect(ctx, cfg)
	if err != nil {
		fmt.Printf("Error executing Pritunl Action Engine: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully configured Pritunl VPN! Connected: %d/%d\n", result.ConnectedCount, result.ExpectedCount)
}
