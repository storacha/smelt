package stack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/storacha/smelt"
)

func TestExtractFiles(t *testing.T) {
	tempDir, err := extractFiles(t)
	if err != nil {
		t.Fatalf("extractFiles failed: %v", err)
	}

	// Verify key files were extracted
	expectedFiles := []string{
		"compose.yml",
		".env",
		"systems/blockchain/compose.yml",
		"systems/blockchain/state/deployed-addresses.json",
		"systems/piri/compose.yml",
		"systems/piri/entrypoint.sh",
		"systems/upload/compose.yml",
		"systems/guppy/compose.yml",
	}

	for _, f := range expectedFiles {
		path := filepath.Join(tempDir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected file %s to exist", f)
		}
	}

	// Verify generated directories were created
	generatedDirs := []string{
		"generated/keys",
		"generated/proofs",
	}

	for _, d := range generatedDirs {
		path := filepath.Join(tempDir, d)
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			t.Errorf("expected directory %s to exist", d)
		} else if !info.IsDir() {
			t.Errorf("expected %s to be a directory", d)
		}
	}
}

func TestGenerateKeys(t *testing.T) {
	tempDir, err := extractFiles(t)
	if err != nil {
		t.Fatalf("extractFiles failed: %v", err)
	}

	err = generateKeys(tempDir)
	if err != nil {
		t.Fatalf("generateKeys failed: %v", err)
	}

	keysDir := filepath.Join(tempDir, "generated", "keys")

	// Verify Ed25519 keys were generated
	for _, svc := range serviceKeys {
		pemPath := filepath.Join(keysDir, svc+".pem")
		if _, err := os.Stat(pemPath); os.IsNotExist(err) {
			t.Errorf("expected key file %s.pem to exist", svc)
		}

		pubPath := filepath.Join(keysDir, svc+".pub")
		if _, err := os.Stat(pubPath); os.IsNotExist(err) {
			t.Errorf("expected public key file %s.pub to exist", svc)
		}
	}

	// Verify EVM keys were generated
	evmKeys := []string{"payer-key.hex", "owner-wallet.hex"}
	for _, k := range evmKeys {
		path := filepath.Join(keysDir, k)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected EVM key file %s to exist", k)
		}
	}
}

func TestEmbeddedFilesExist(t *testing.T) {
	// Verify we can read key embedded files
	files := []string{
		"compose.yml",
		".env",
		"systems/blockchain/state/deployed-addresses.json",
	}

	for _, f := range files {
		data, err := smelt.EmbeddedFiles.ReadFile(f)
		if err != nil {
			t.Errorf("failed to read embedded file %s: %v", f, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("embedded file %s is empty", f)
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()

	if cfg.timeout == 0 {
		t.Error("expected default timeout to be non-zero")
	}

	// Verify buildEnv with no overrides returns empty map
	env := cfg.buildEnv()
	if len(env) != 0 {
		t.Errorf("expected empty env map with no overrides, got %d entries", len(env))
	}

	// Verify buildEnv with overrides
	cfg.piriImage = "test-piri:latest"
	cfg.guppyImage = "test-guppy:latest"
	env = cfg.buildEnv()

	if env["PIRI_IMAGE"] != "test-piri:latest" {
		t.Errorf("expected PIRI_IMAGE=test-piri:latest, got %s", env["PIRI_IMAGE"])
	}
	if env["GUPPY_IMAGE"] != "test-guppy:latest" {
		t.Errorf("expected GUPPY_IMAGE=test-guppy:latest, got %s", env["GUPPY_IMAGE"])
	}
}

func TestOptions(t *testing.T) {
	cfg := defaultConfig()

	WithPiriImage("my-piri:v1")(cfg)
	if cfg.piriImage != "my-piri:v1" {
		t.Errorf("WithPiriImage failed: got %s", cfg.piriImage)
	}

	WithGuppyImage("my-guppy:v1")(cfg)
	if cfg.guppyImage != "my-guppy:v1" {
		t.Errorf("WithGuppyImage failed: got %s", cfg.guppyImage)
	}

	WithKeepOnFailure()(cfg)
	if !cfg.keepOnFailure {
		t.Error("WithKeepOnFailure failed")
	}
}
