package manifest

import (
	"testing"
)

func TestParseCountForm(t *testing.T) {
	data := []byte(`
version: 1
piri:
  count: 3
  defaults:
    storage:
      db: postgres
      blob: s3
`)
	m, err := ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := m.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(nodes))
	}
	for i, n := range nodes {
		if n.Storage.DB != DBPostgres {
			t.Errorf("node %d: expected db=%q, got %q", i, DBPostgres, n.Storage.DB)
		}
		if n.Storage.Blob != BlobS3 {
			t.Errorf("node %d: expected blob=%q, got %q", i, BlobS3, n.Storage.Blob)
		}
		if n.Name != "piri-"+string(rune('0'+i)) {
			t.Errorf("node %d: expected name piri-%d, got %q", i, i, n.Name)
		}
	}
}

func TestParseNodesForm(t *testing.T) {
	data := []byte(`
version: 1
piri:
  defaults:
    storage:
      db: sqlite
      blob: filesystem
  nodes:
    - name: piri-0
    - name: piri-1
      storage:
        db: postgres
        blob: s3
    - name: piri-2
      image: ghcr.io/storacha/piri:test
      storage:
        db: postgres
`)
	m, err := ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := m.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(nodes))
	}

	// piri-0: inherits all defaults
	if nodes[0].Storage.DB != DBSQLite || nodes[0].Storage.Blob != BlobFS {
		t.Errorf("piri-0: expected sqlite/filesystem, got %s/%s", nodes[0].Storage.DB, nodes[0].Storage.Blob)
	}

	// piri-1: full override
	if nodes[1].Storage.DB != DBPostgres || nodes[1].Storage.Blob != BlobS3 {
		t.Errorf("piri-1: expected postgres/s3, got %s/%s", nodes[1].Storage.DB, nodes[1].Storage.Blob)
	}

	// piri-2: partial override (db=postgres, blob inherits filesystem)
	if nodes[2].Storage.DB != DBPostgres || nodes[2].Storage.Blob != BlobFS {
		t.Errorf("piri-2: expected postgres/filesystem, got %s/%s", nodes[2].Storage.DB, nodes[2].Storage.Blob)
	}
	if nodes[2].Image != "ghcr.io/storacha/piri:test" {
		t.Errorf("piri-2: expected image override, got %q", nodes[2].Image)
	}
}

func TestParseDefault(t *testing.T) {
	data := []byte(`
version: 1
piri:
  defaults:
    storage:
      db: sqlite
      blob: filesystem
  nodes:
    - name: piri-0
`)
	m, err := ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := m.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].Name != "piri-0" {
		t.Errorf("expected name piri-0, got %q", nodes[0].Name)
	}
}

func TestParseEmpty(t *testing.T) {
	data := []byte(`
version: 1
piri: {}
`)
	m, err := ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := m.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 default node, got %d", len(nodes))
	}
	if nodes[0].Name != "piri-0" {
		t.Errorf("expected name piri-0, got %q", nodes[0].Name)
	}
	if nodes[0].Storage.DB != DBSQLite {
		t.Errorf("expected db=sqlite, got %q", nodes[0].Storage.DB)
	}
}

func TestErrorBothCountAndNodes(t *testing.T) {
	data := []byte(`
version: 1
piri:
  count: 2
  nodes:
    - name: piri-0
`)
	m, err := ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.Resolve()
	if err == nil {
		t.Fatal("expected error when both count and nodes specified")
	}
}

func TestErrorTooManyNodes(t *testing.T) {
	data := []byte(`
version: 1
piri:
  count: 10
`)
	m, err := ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.Resolve()
	if err == nil {
		t.Fatal("expected error for 10 nodes")
	}
}

func TestErrorDuplicateNames(t *testing.T) {
	data := []byte(`
version: 1
piri:
  nodes:
    - name: piri-0
    - name: piri-0
`)
	m, err := ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.Resolve()
	if err == nil {
		t.Fatal("expected error for duplicate names")
	}
}

func TestErrorInvalidDB(t *testing.T) {
	data := []byte(`
version: 1
piri:
  nodes:
    - storage:
        db: mysql
`)
	m, err := ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.Resolve()
	if err == nil {
		t.Fatal("expected error for invalid db backend")
	}
}

func TestAutoGenerateNames(t *testing.T) {
	data := []byte(`
version: 1
piri:
  nodes:
    - {}
    - {}
    - {}
`)
	m, err := ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := m.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	for i, n := range nodes {
		expected := "piri-" + string(rune('0'+i))
		if n.Name != expected {
			t.Errorf("node %d: expected name %q, got %q", i, expected, n.Name)
		}
	}
}
