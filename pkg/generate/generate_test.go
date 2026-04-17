package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/storacha/smelt/pkg/manifest"
	"gopkg.in/yaml.v3"
)

func TestGenerateKeys(t *testing.T) {
	keysDir := t.TempDir()
	nodes := []manifest.ResolvedPiriNode{
		{Name: "piri-0", Index: 0, Storage: manifest.StorageSpec{DB: "sqlite", Blob: "filesystem"}},
		{Name: "piri-1", Index: 1, Storage: manifest.StorageSpec{DB: "postgres", Blob: "s3"}},
	}

	if err := GenerateKeys(keysDir, nodes, false); err != nil {
		t.Fatal(err)
	}

	// Check piri keys exist.
	for _, name := range []string{"piri-0", "piri-1"} {
		for _, ext := range []string{".pem", ".pub"} {
			path := filepath.Join(keysDir, name+ext)
			if _, err := os.Stat(path); err != nil {
				t.Errorf("expected %s to exist", path)
			}
		}
	}

	// Check wallets exist.
	for _, name := range []string{"piri-0-wallet", "piri-1-wallet"} {
		path := filepath.Join(keysDir, name+".hex")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected %s to exist", path)
		}
	}

	// Check non-piri service keys.
	for _, svc := range nonPiriServiceKeys {
		path := filepath.Join(keysDir, svc+".pem")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected %s to exist", path)
		}
	}

	// Check payer key.
	payerPath := filepath.Join(keysDir, "payer-key.hex")
	if _, err := os.Stat(payerPath); err != nil {
		t.Error("expected payer-key.hex to exist")
	}
}

func TestGenerateKeysIdempotent(t *testing.T) {
	keysDir := t.TempDir()
	nodes := []manifest.ResolvedPiriNode{
		{Name: "piri-0", Index: 0, Storage: manifest.StorageSpec{DB: "sqlite", Blob: "filesystem"}},
	}

	if err := GenerateKeys(keysDir, nodes, false); err != nil {
		t.Fatal(err)
	}

	// Read key content.
	keyPath := filepath.Join(keysDir, "piri-0.pem")
	original, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}

	// Re-generate without force — key should not change.
	if err := GenerateKeys(keysDir, nodes, false); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(original) != string(after) {
		t.Error("key was regenerated without force flag")
	}
}

func TestGeneratePiriComposeSingleNode(t *testing.T) {
	nodes := []manifest.ResolvedPiriNode{
		{Name: "piri-0", Index: 0, Storage: manifest.StorageSpec{DB: "sqlite", Blob: "filesystem"}},
	}

	data, err := GeneratePiriCompose(nodes)
	if err != nil {
		t.Fatal(err)
	}

	yaml := string(data)

	// Should contain piri-0 service.
	if !strings.Contains(yaml, "piri-0:") {
		t.Error("expected piri-0 service in compose")
	}

	// Should NOT contain postgres or minio (sqlite/filesystem).
	if strings.Contains(yaml, "piri-postgres:") {
		t.Error("postgres service should not be present for sqlite-only config")
	}
	if strings.Contains(yaml, "piri-minio:") {
		t.Error("minio service should not be present for filesystem-only config")
	}

	// Base compose should NOT contain port mappings (ports are in the separate ports file).
	if strings.Contains(yaml, "4000:3000") {
		t.Error("base compose should not contain port mappings")
	}
}

func TestGeneratePiriComposeMultiNode(t *testing.T) {
	nodes := []manifest.ResolvedPiriNode{
		{Name: "piri-0", Index: 0, Storage: manifest.StorageSpec{DB: "sqlite", Blob: "filesystem"}},
		{Name: "piri-1", Index: 1, Storage: manifest.StorageSpec{DB: "postgres", Blob: "s3"}},
		{Name: "piri-2", Index: 2, Storage: manifest.StorageSpec{DB: "postgres", Blob: "filesystem"}},
	}

	data, err := GeneratePiriCompose(nodes)
	if err != nil {
		t.Fatal(err)
	}

	yaml := string(data)

	// All three services should be present.
	for _, name := range []string{"piri-0:", "piri-1:", "piri-2:"} {
		if !strings.Contains(yaml, name) {
			t.Errorf("expected %s service in compose", name)
		}
	}

	// Base compose should NOT contain port mappings.
	for _, port := range []string{"4000:3000", "4001:3000", "4002:3000"} {
		if strings.Contains(yaml, port) {
			t.Errorf("base compose should not contain port mapping %s", port)
		}
	}

	// Postgres service should be present (piri-1 and piri-2 use it).
	if !strings.Contains(yaml, "piri-postgres:") {
		t.Error("expected piri-postgres service")
	}
	if !strings.Contains(yaml, "piri-postgres-init:") {
		t.Error("expected piri-postgres-init service")
	}

	// MinIO should be present (piri-1 uses S3).
	if !strings.Contains(yaml, "piri-minio:") {
		t.Error("expected piri-minio service")
	}

	// piri-0 should NOT have postgres URL.
	// Check that piri-1 has its postgres URL.
	if !strings.Contains(yaml, "piri_1?sslmode=disable") {
		t.Error("expected postgres URL for piri_1")
	}
	if !strings.Contains(yaml, "piri_2?sslmode=disable") {
		t.Error("expected postgres URL for piri_2")
	}

	// Bucket prefix.
	if !strings.Contains(yaml, "PIRI_S3_BUCKET_PREFIX=piri-1-") {
		t.Error("expected S3 bucket prefix for piri-1")
	}

	// piri-0 key mount.
	if !strings.Contains(yaml, "../keys/piri-0.pem:/keys/piri.pem:ro") {
		t.Error("expected piri-0 key mount")
	}
	if !strings.Contains(yaml, "../keys/piri-1-wallet.hex:/keys/owner-wallet.hex:ro") {
		t.Error("expected piri-1 wallet mount")
	}
}

func TestGeneratePiriComposeSerializedStartup(t *testing.T) {
	nodes := []manifest.ResolvedPiriNode{
		{Name: "piri-0", Index: 0, Storage: manifest.StorageSpec{DB: "sqlite", Blob: "filesystem"}},
		{Name: "piri-1", Index: 1, Storage: manifest.StorageSpec{DB: "sqlite", Blob: "filesystem"}},
		{Name: "piri-2", Index: 2, Storage: manifest.StorageSpec{DB: "sqlite", Blob: "filesystem"}},
	}

	data, err := GeneratePiriCompose(nodes)
	if err != nil {
		t.Fatal(err)
	}

	var compose ComposeFile
	if err := yaml.Unmarshal(data, &compose); err != nil {
		t.Fatalf("unmarshal generated compose: %v", err)
	}

	// piri-0 should NOT depend on any other piri node.
	if dep, ok := compose.Services["piri-0"].DependsOn["piri-0"]; ok {
		t.Errorf("piri-0 should not depend on itself: %+v", dep)
	}

	// piri-1 should depend on piri-0 being healthy.
	if dep, ok := compose.Services["piri-1"].DependsOn["piri-0"]; !ok {
		t.Error("piri-1 should depend on piri-0")
	} else if dep.Condition != "service_healthy" {
		t.Errorf("piri-1 -> piri-0 condition = %q, want service_healthy", dep.Condition)
	}

	// piri-2 should depend on piri-1 being healthy (chain), not piri-0 directly.
	if dep, ok := compose.Services["piri-2"].DependsOn["piri-1"]; !ok {
		t.Error("piri-2 should depend on piri-1")
	} else if dep.Condition != "service_healthy" {
		t.Errorf("piri-2 -> piri-1 condition = %q, want service_healthy", dep.Condition)
	}
	if _, ok := compose.Services["piri-2"].DependsOn["piri-0"]; ok {
		t.Error("piri-2 should depend on piri-1 only, not piri-0 directly")
	}
}

func TestGeneratePiriPortsComposeSingleNode(t *testing.T) {
	nodes := []manifest.ResolvedPiriNode{
		{Name: "piri-0", Index: 0, Storage: manifest.StorageSpec{DB: "sqlite", Blob: "filesystem"}},
	}

	data, err := GeneratePiriPortsCompose(nodes)
	if err != nil {
		t.Fatal(err)
	}

	yaml := string(data)

	// Should contain the correct port mapping.
	if !strings.Contains(yaml, "4000:3000") {
		t.Error("expected port mapping 4000:3000")
	}

	// Should NOT contain postgres or minio ports (sqlite/filesystem).
	if strings.Contains(yaml, "5432:5432") {
		t.Error("postgres port should not be present for sqlite-only config")
	}
	if strings.Contains(yaml, "9002:9000") {
		t.Error("minio port should not be present for filesystem-only config")
	}
}

func TestGeneratePiriPortsComposeMultiNode(t *testing.T) {
	nodes := []manifest.ResolvedPiriNode{
		{Name: "piri-0", Index: 0, Storage: manifest.StorageSpec{DB: "sqlite", Blob: "filesystem"}},
		{Name: "piri-1", Index: 1, Storage: manifest.StorageSpec{DB: "postgres", Blob: "s3"}},
		{Name: "piri-2", Index: 2, Storage: manifest.StorageSpec{DB: "postgres", Blob: "filesystem"}},
	}

	data, err := GeneratePiriPortsCompose(nodes)
	if err != nil {
		t.Fatal(err)
	}

	yaml := string(data)

	// All piri port mappings should be present.
	for _, port := range []string{"4000:3000", "4001:3000", "4002:3000"} {
		if !strings.Contains(yaml, port) {
			t.Errorf("expected port mapping %s", port)
		}
	}

	// Postgres port should be present.
	if !strings.Contains(yaml, "5432:5432") {
		t.Error("expected postgres port mapping")
	}

	// MinIO ports should be present.
	if !strings.Contains(yaml, "9002:9000") {
		t.Error("expected minio S3 port mapping")
	}
	if !strings.Contains(yaml, "9003:9001") {
		t.Error("expected minio console port mapping")
	}

	// Ports file should NOT contain non-port configuration.
	if strings.Contains(yaml, "healthcheck") {
		t.Error("ports file should not contain healthcheck configuration")
	}
	if strings.Contains(yaml, "volumes") {
		t.Error("ports file should not contain volume configuration")
	}
}

func TestGenerateEndToEnd(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a manifest.
	manifestPath := filepath.Join(tmpDir, "smelt.yml")
	manifestContent := `
version: 1
piri:
  nodes:
    - name: piri-0
      storage:
        db: sqlite
        blob: filesystem
    - name: piri-1
      storage:
        db: postgres
        blob: s3
`
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := Generate(Options{
		ManifestPath: manifestPath,
		ProjectDir:   tmpDir,
		Force:        false,
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.NodeCount != 2 {
		t.Errorf("expected 2 nodes, got %d", result.NodeCount)
	}

	// Check compose files exist.
	if _, err := os.Stat(result.PiriComposePath); err != nil {
		t.Errorf("piri compose not created: %v", err)
	}
	if _, err := os.Stat(result.PiriPortsComposePath); err != nil {
		t.Errorf("piri ports compose not created: %v", err)
	}

	// Check keys exist.
	for _, name := range []string{"piri-0.pem", "piri-1.pem", "piri-0-wallet.hex", "piri-1-wallet.hex", "payer-key.hex"} {
		path := filepath.Join(result.KeysDir, name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected %s to exist", name)
		}
	}
}

func TestPiriAccountIndex(t *testing.T) {
	tests := []struct {
		piriIndex int
		expected  int
	}{
		{0, 0}, // piri-0 uses account 0 (deployer)
		{1, 2}, // piri-1 uses account 2 (skip account 1 = payer)
		{2, 3},
		{3, 4},
		{8, 9}, // piri-8 uses account 9 (last available)
	}
	for _, tt := range tests {
		got := PiriAccountIndex(tt.piriIndex)
		if got != tt.expected {
			t.Errorf("PiriAccountIndex(%d) = %d, want %d", tt.piriIndex, got, tt.expected)
		}
	}
}
