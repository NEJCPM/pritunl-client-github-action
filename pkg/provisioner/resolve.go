package provisioner

import (
	"os"
	"runtime"
)

// ResolveClientBinaryPath returns the path to the installed pritunl-client
// binary. Windows installers deploy under Program Files without refreshable
// shims, so the resolved absolute path is used instead of a PATH lookup.
func ResolveClientBinaryPath() string {
	if runtime.GOOS != "windows" {
		return "pritunl-client"
	}
	candidates := []string{
		`C:\Program Files (x86)\Pritunl\pritunl-client.exe`,
		`C:\Program Files\Pritunl\pritunl-client.exe`,
	}
	for _, candidate := range candidates {
		if fileExists(candidate) {
			return candidate
		}
	}
	return "pritunl-client"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
