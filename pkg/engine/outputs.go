package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/NEJCPM/pritunl-client-github-action/pkg/domain"
)

// WriteActionOutputs writes client-id and client-ids to GitHub Actions output file ($GITHUB_OUTPUT).
func WriteActionOutputs(outputPath string, primaryID string, allOutputs []domain.ClientIDOutput, concealed bool) error {
	if !isValidOutputValue(primaryID) {
		return fmt.Errorf("primary client id %q contains newline or control characters and cannot be written to GITHUB_OUTPUT", primaryID)
	}
	for _, o := range allOutputs {
		if !isValidOutputValue(o.ID) || !isValidOutputValue(o.Name) {
			return fmt.Errorf("client output for id %q contains newline or control characters and cannot be written to GITHUB_OUTPUT", o.ID)
		}
	}

	allJSON, err := json.Marshal(allOutputs)
	if err != nil {
		return fmt.Errorf("failed to marshal all client ids JSON: %w", err)
	}

	if outputPath != "" {
		f, err := os.OpenFile(outputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open GITHUB_OUTPUT file %s: %w", outputPath, err)
		}
		defer f.Close()

		if _, err := fmt.Fprintf(f, "client-id=%s\n", primaryID); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(f, "client-ids=%s\n", string(allJSON)); err != nil {
			return err
		}
	}

	if !concealed {
		fmt.Printf("Action Step Outputs:\nclient-id: %s\nclient-ids: %s\n", primaryID, string(allJSON))
	} else {
		fmt.Println("Step outputs are concealed. Set concealed-outputs to false in action inputs to reveal.")
	}

	return nil
}

// isValidOutputValue rejects values that would inject extra lines into the
// key=value format of the GITHUB_OUTPUT file.
func isValidOutputValue(val string) bool {
	return !strings.ContainsFunc(val, func(r rune) bool {
		return r == '\n' || r == '\r' || unicode.IsControl(r)
	})
}
