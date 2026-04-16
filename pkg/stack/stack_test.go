package stack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/storacha/smelt"
	"github.com/storacha/smelt/pkg/generate"
	"github.com/storacha/smelt/pkg/manifest"
)

func TestExtractFiles(t *testing.T) {
	tempDir, err := extractFiles(t)
	if err != nil {
		t.Fatalf("extractFiles failed: %v", err)
	}

	expectedFiles := []string{
		"compose.yml",
		".env",
		"systems/blockchain/compose.yml",
		"systems/blockchain/state/deployed-addresses.json",
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
	keysDir := t.TempDir()
	nodes := []manifest.ResolvedPiriNode{
		{Name: "piri-0", Index: 0, Storage: manifest.StorageSpec{DB: "sqlite", Blob: "filesystem"}},
	}

	err := generate.GenerateKeys(keysDir, nodes, false)
	if err != nil {
		t.Fatalf("GenerateKeys failed: %v", err)
	}

	// Verify Ed25519 keys were generated for piri-0.
	for _, ext := range []string{".pem", ".pub"} {
		path := filepath.Join(keysDir, "piri-0"+ext)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected piri-0%s to exist", ext)
		}
	}

	// Verify non-piri service keys.
	for _, svc := range []string{"upload", "indexer", "delegator", "signing-service", "etracker"} {
		path := filepath.Join(keysDir, svc+".pem")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected %s.pem to exist", svc)
		}
	}

	// Verify EVM keys.
	for _, k := range []string{"payer-key.hex", "piri-0-wallet.hex"} {
		path := filepath.Join(keysDir, k)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected %s to exist", k)
		}
	}
}

func TestEmbeddedFilesExist(t *testing.T) {
	files := []string{
		"compose.yml",
		".env",
		"systems/blockchain/state/deployed-addresses.json",
		"systems/piri/config/piri-base-config.toml",
		"systems/piri/config/piri-overrides.toml",
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

	env := cfg.buildEnv()
	if len(env) != 0 {
		t.Errorf("expected empty env map with no overrides, got %d entries", len(env))
	}

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

func TestResolveNodes(t *testing.T) {
	t.Run("DefaultSingleNode", func(t *testing.T) {
		cfg := defaultConfig()
		nodes := cfg.resolveNodes()
		if len(nodes) != 1 {
			t.Fatalf("expected 1 node, got %d", len(nodes))
		}
		if nodes[0].Name != "piri-0" {
			t.Errorf("expected piri-0, got %s", nodes[0].Name)
		}
		if nodes[0].Storage.DB != manifest.DBSQLite {
			t.Errorf("expected sqlite, got %s", nodes[0].Storage.DB)
		}
	})

	t.Run("WithPiriCount", func(t *testing.T) {
		cfg := defaultConfig()
		WithPiriCount(3)(cfg)
		nodes := cfg.resolveNodes()
		if len(nodes) != 3 {
			t.Fatalf("expected 3 nodes, got %d", len(nodes))
		}
		for i, n := range nodes {
			if n.Name != "piri-"+string(rune('0'+i)) {
				t.Errorf("node %d: expected piri-%d, got %s", i, i, n.Name)
			}
		}
	})

	t.Run("WithPiriNodes", func(t *testing.T) {
		cfg := defaultConfig()
		WithPiriNodes(
			PiriNodeConfig{Postgres: true, S3: true},
			PiriNodeConfig{},
		)(cfg)
		nodes := cfg.resolveNodes()
		if len(nodes) != 2 {
			t.Fatalf("expected 2 nodes, got %d", len(nodes))
		}
		if nodes[0].Storage.DB != manifest.DBPostgres || nodes[0].Storage.Blob != manifest.BlobS3 {
			t.Errorf("node 0: expected postgres/s3, got %s/%s", nodes[0].Storage.DB, nodes[0].Storage.Blob)
		}
		if nodes[1].Storage.DB != manifest.DBSQLite || nodes[1].Storage.Blob != manifest.BlobFS {
			t.Errorf("node 1: expected sqlite/filesystem, got %s/%s", nodes[1].Storage.DB, nodes[1].Storage.Blob)
		}
	})
}
