package checksum

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadManifest(t *testing.T) {
	m, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if m.Schema != 1 {
		t.Errorf("schema = %d, want 1", m.Schema)
	}
	for _, version := range []string{"1.3.4696.56", "1.3.3883.60"} {
		entry, ok := m.Artifacts[version]
		if !ok {
			t.Fatalf("manifest missing version %s", version)
		}
		for _, key := range []string{"deb-noble-amd64", "deb-jammy-amd64", "pkg-zip", "exe"} {
			digest := entry[key]
			if len(digest) != 64 {
				t.Errorf("version %s key %s digest malformed: %q", version, key, digest)
			}
			for i := 0; i < len(digest); i++ {
				c := digest[i]
				if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
					t.Errorf("version %s key %s digest not lowercase hex", version, key)
				}
			}
		}
	}
}

func TestExpected_Policy(t *testing.T) {
	m := &Manifest{
		Schema: 1,
		Artifacts: map[string]map[string]string{
			"1.2.3": {"deb-noble-amd64": strings.Repeat("a", 64)},
		},
	}

	tests := []struct {
		name      string
		version   string
		key       string
		clientSHA string
		want      string
		wantErr   string
	}{
		{name: "pinned version uses manifest", version: "1.2.3", key: "deb-noble-amd64", want: strings.Repeat("a", 64)},
		{name: "caller input overrides manifest", version: "1.2.3", key: "deb-noble-amd64", clientSHA: strings.Repeat("b", 64), want: strings.Repeat("b", 64)},
		{name: "caller input accepted for unpinned", version: "9.9.9", key: "exe", clientSHA: strings.Repeat("c", 64), want: strings.Repeat("c", 64)},
		{name: "unpinned without input rejected", version: "9.9.9", key: "exe", wantErr: "no pinned checksum"},
		{name: "pinned without digest rejected", version: "1.2.3", key: "exe", wantErr: "no digest for"},
		{name: "invalid input ignored then rejected", version: "9.9.9", key: "exe", clientSHA: "xyz", wantErr: "no pinned checksum"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := m.Expected(tt.version, tt.key, tt.clientSHA)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Expected() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestExpected_UppercaseInputNormalized(t *testing.T) {
	m := &Manifest{Schema: 1, Artifacts: map[string]map[string]string{}}
	upper := strings.ToUpper(strings.Repeat("a", 64))

	got, err := m.Expected("9.9.9", "exe", upper)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != strings.Repeat("a", 64) {
		t.Errorf("uppercase input not normalized: %s", got)
	}
}

func TestVerifyFile(t *testing.T) {
	content := []byte("pritunl e2e artifact content")
	sum := sha256.Sum256(content)
	digest := hex.EncodeToString(sum[:])

	path := filepath.Join(t.TempDir(), "artifact.bin")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}

	if err := VerifyFile(path, digest); err != nil {
		t.Errorf("VerifyFile with matching digest failed: %v", err)
	}
	if err := VerifyFile(path, strings.ToUpper(digest)); err != nil {
		t.Errorf("VerifyFile with uppercase digest failed: %v", err)
	}
	err := VerifyFile(path, strings.Repeat("0", 64))
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("expected mismatch error, got: %v", err)
	}
	if err := VerifyFile(path, "not-a-sha"); err == nil || !strings.Contains(err.Error(), "invalid expected sha256") {
		t.Errorf("expected invalid digest error, got: %v", err)
	}
	if err := VerifyFile(filepath.Join(t.TempDir(), "missing.bin"), digest); err == nil || !strings.Contains(err.Error(), "failed to open artifact") {
		t.Errorf("expected open error, got: %v", err)
	}
}