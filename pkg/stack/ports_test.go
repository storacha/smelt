package stack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStripHostPorts(t *testing.T) {
	input := `services:
  blockchain:
    ports:
      - "8545:8545"
  email:
    ports:
      - "2525:25"
      - "2580:80"   # Web UI / API
  upload:
    environment:
      - "SPRUE_SERVER_PUBLIC_URL=http://upload:80"
    ports:
      - "8080:80"
`
	expected := `services:
  blockchain:
    ports:
      - "8545"
  email:
    ports:
      - "25"
      - "80"   # Web UI / API
  upload:
    environment:
      - "SPRUE_SERVER_PUBLIC_URL=http://upload:80"
    ports:
      - "80"
`

	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "compose.yml")
	if err := os.WriteFile(composePath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	if err := stripHostPortsInDir(tempDir); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != expected {
		t.Errorf("unexpected rewrite:\nwant:\n%s\ngot:\n%s", expected, string(got))
	}
}

func TestStripHostPortsUnquoted(t *testing.T) {
	// yaml.v3 marshals plain numeric-colon-numeric strings without quotes,
	// which is what the generator emits into piri.yml. The rewriter has to
	// handle this form too — if it doesn't, parallel test stacks collide on
	// the fixed piri host ports (15100, 15101, ...).
	input := `services:
  piri-0:
    ports:
        - 15100:3000
  piri-1:
    ports:
        - 15101:3000
  piri-minio:
    ports:
        - 15072:9000
        - 15073:9001
`
	expected := `services:
  piri-0:
    ports:
        - 3000
  piri-1:
    ports:
        - 3000
  piri-minio:
    ports:
        - 9000
        - 9001
`

	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "piri.yml")
	if err := os.WriteFile(path, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	if err := stripHostPortsInDir(tempDir); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != expected {
		t.Errorf("unexpected rewrite:\nwant:\n%s\ngot:\n%s", expected, string(got))
	}
}

func TestRewriteRootNetwork(t *testing.T) {
	input := `include:
  - path: systems/blockchain/compose.yml

networks:
  storacha-network:
    external: true
`
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "compose.yml"), []byte(input), 0644); err != nil {
		t.Fatal(err)
	}
	if err := rewriteRootNetwork(tempDir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(tempDir, "compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "external: true") {
		t.Errorf("expected external: true to be removed, got:\n%s", string(got))
	}
	if !strings.Contains(string(got), "storacha-network: {}") {
		t.Errorf("expected project-scoped network declaration, got:\n%s", string(got))
	}
	// Includes should be left intact.
	if !strings.Contains(string(got), "systems/blockchain/compose.yml") {
		t.Errorf("unrelated content dropped during rewrite:\n%s", string(got))
	}
}

func TestRewriteSprueConfig(t *testing.T) {
	input := `server:
  host: "0.0.0.0"
  port: 80
  public_url: http://localhost:8080

identity:
  key_file: "/keys/upload.pem"
`
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "systems", "upload", "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(path, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	if err := rewriteSprueConfig(tempDir); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "public_url: http://upload:80") {
		t.Errorf("expected in-network public_url, got:\n%s", string(got))
	}
	// Other fields should be untouched.
	if !strings.Contains(string(got), `key_file: "/keys/upload.pem"`) {
		t.Errorf("unrelated content dropped during rewrite:\n%s", string(got))
	}
}
