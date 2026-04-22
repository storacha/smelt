package stack

import (
	"fmt"

	"github.com/storacha/smelt/pkg/manifest"
)

// testModeEnv returns the SMELT_* environment variables pkg/stack sets when
// provisioning a stack for SDK tests. They feed the `${VAR:-default}`
// interpolations baked into the compose files (see systems/*/compose.yml and
// pkg/generate/compose.go), flipping every host-side port mapping to
// ephemeral and marking the storacha-network as non-external.
//
// Setting a port var to just the container-side number (no colon) makes
// compose publish on an ephemeral host port; MappedPort on the resulting
// container resolves it back to whatever Docker picked. `make up` leaves
// these unset and gets the fixed 15XXX bindings from the defaults.
func testModeEnv(nodes []manifest.ResolvedPiriNode) map[string]string {
	env := map[string]string{
		// Network: per-project (compose-created) instead of the shared
		// external `storacha-network`. Parallel test stacks otherwise
		// collide on service DNS names inside the one shared bridge.
		"SMELT_NETWORK_EXTERNAL": "false",

		// Sprue public_url → in-network. The host can't reach a stable
		// port on sprue anymore (ephemeral), so validation emails embed
		// `http://upload:80` and the ExecDoer-based clicker POSTs back
		// from inside the guppy container.
		"SPRUE_SERVER_PUBLIC_URL": "http://upload:80",

		// Every port var set to the container-side number only → compose
		// publishes an ephemeral host port. Values mirror each service's
		// container-internal port (see the defaults in the corresponding
		// compose file).
		"SMELT_BLOCKCHAIN_PORT":      "8545",
		"SMELT_DYNAMODB_PORT":        "8000",
		"SMELT_MINIO_S3_PORT":        "9000",
		"SMELT_MINIO_CONSOLE_PORT":   "9001",
		"SMELT_SMTP_PORT":            "25",
		"SMELT_SMTP_WEB_PORT":        "80",
		"SMELT_SIGNING_SERVICE_PORT": "7446",
		"SMELT_DELEGATOR_PORT":       "80",
		"SMELT_REDIS_PORT":           "6379",
		"SMELT_INDEXER_PORT":         "80",
		"SMELT_IPNI_FINDER_PORT":     "3000",
		"SMELT_IPNI_ADMIN_PORT":      "3002",
		"SMELT_IPNI_P2P_PORT":        "3003",
		"SMELT_UPLOAD_PORT":          "80",

		// Piri shared infra — only used when any node declares postgres/s3,
		// but harmless to set unconditionally (compose ignores unknown
		// substitutions for services not in the stack).
		"SMELT_PIRI_POSTGRES_PORT":      "5432",
		"SMELT_PIRI_MINIO_S3_PORT":      "9000",
		"SMELT_PIRI_MINIO_CONSOLE_PORT": "9001",
	}

	// Per-node piri ports (SMELT_PIRI_0_PORT, SMELT_PIRI_1_PORT, ...).
	// Generator-emitted piri.yml references these by node index.
	for _, node := range nodes {
		env[fmt.Sprintf("SMELT_PIRI_%d_PORT", node.Index)] = "3000"
	}

	return env
}
