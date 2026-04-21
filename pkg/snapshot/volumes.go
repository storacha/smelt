package snapshot

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/storacha/smelt/pkg/manifest"
)

// busyboxImage is the helper used to tar/untar through docker volumes.
// It's tiny and ubiquitous — any host running smelt already has it pulled
// (or gets it in seconds on first use).
const busyboxImage = "busybox:latest"

// resolveVolumes returns the compose-declared volume names (without project
// prefix) that this manifest produces. Only volumes actually created by the
// resolved topology are returned — e.g. piri-postgres-data only appears when
// at least one node is configured with the postgres backend.
func resolveVolumes(m *manifest.Manifest) ([]string, error) {
	nodes, err := m.Resolve()
	if err != nil {
		return nil, fmt.Errorf("resolve manifest: %w", err)
	}

	// Always-present volumes declared by non-piri systems. Keeping the full
	// set (including guppy client state) matches the "snapshot is the whole
	// stack delta" framing — a user who logged in and created spaces before
	// saving will find them after load.
	vols := []string{
		"minio-data",    // systems/common — upload's S3 backend
		"ipni-data",     // systems/indexing/ipni — content discovery
		"dynamodb-data", // systems/common — delegator allow list, upload registry
		"guppy-data",    // systems/guppy — client's login/space state
	}

	// Per-piri data volumes.
	hasPostgres := false
	hasS3 := false
	for _, n := range nodes {
		vols = append(vols, fmt.Sprintf("%s-data", n.Name))
		if n.Storage.DB == manifest.DBPostgres {
			hasPostgres = true
		}
		if n.Storage.Blob == manifest.BlobS3 {
			hasS3 = true
		}
	}
	if hasPostgres {
		vols = append(vols, "piri-postgres-data")
	}
	if hasS3 {
		vols = append(vols, "piri-minio-data")
	}

	return vols, nil
}

// archiveVolume tars the contents of a docker-named volume into outputDir
// as `<volname>.tar`. The tar is rooted at the volume contents (`.`) so
// restore can extract directly into a fresh volume mount.
//
// Runs busybox as root so postgres/minio data (root-owned inside the volume)
// is readable.
func archiveVolume(ctx context.Context, projectName, volName, outputDir string) error {
	fullVol := fmt.Sprintf("%s_%s", projectName, volName)

	outputDirAbs, err := filepath.Abs(outputDir)
	if err != nil {
		return fmt.Errorf("resolve output dir: %w", err)
	}
	if err := os.MkdirAll(outputDirAbs, 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"-u", "0:0",
		"-v", fmt.Sprintf("%s:/src:ro", fullVol),
		"-v", fmt.Sprintf("%s:/dst", outputDirAbs),
		busyboxImage,
		"tar", "-C", "/src", "-cf", fmt.Sprintf("/dst/%s.tar", volName), ".",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// Volume-not-found is non-fatal — the manifest may list volumes that
		// this particular session never created (e.g. piri-postgres-data when
		// the stack was brought up with only sqlite nodes during dev, then
		// the manifest was edited). Surface the error for other causes.
		if !volumeExists(ctx, fullVol) {
			fmt.Fprintf(os.Stderr, "  skip: volume %q does not exist\n", fullVol)
			return nil
		}
		return fmt.Errorf("archive %s: %w", fullVol, err)
	}
	return nil
}

// RestoreVolume overwrites the contents of a docker-named volume
// (`<projectName>_<volName>`) from a tar produced by archiveVolume.
// Always rm-then-create the volume so it carries the compose labels —
// otherwise `make up` warns "already exists but was not created by
// Docker Compose" on every restore. Docker doesn't let us add labels to
// an existing volume; only set them at create.
//
// Exported for use by pkg/stack, which pre-populates per-test volumes
// before the testcontainers-go compose.Up(). The compose layer's Load
// also uses it for the make-up path.
func RestoreVolume(ctx context.Context, projectName, volName, inputDir string) error {
	fullVol := fmt.Sprintf("%s_%s", projectName, volName)

	inputDirAbs, err := filepath.Abs(inputDir)
	if err != nil {
		return fmt.Errorf("resolve input dir: %w", err)
	}
	tarName := fmt.Sprintf("%s.tar", volName)
	tarPath := filepath.Join(inputDirAbs, tarName)
	if _, err := os.Stat(tarPath); err != nil {
		// The snapshot didn't include this volume (e.g. saved with sqlite-only
		// topology, loading into a postgres topology). Skip rather than fail;
		// the first `make up` will populate an empty volume fresh.
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "  skip: no archive for %q in snapshot\n", volName)
			return nil
		}
		return fmt.Errorf("stat tar %s: %w", tarPath, err)
	}

	if volumeExists(ctx, fullVol) {
		rm := exec.CommandContext(ctx, "docker", "volume", "rm", fullVol)
		rm.Stderr = os.Stderr
		if err := rm.Run(); err != nil {
			return fmt.Errorf("remove existing volume %s: %w", fullVol, err)
		}
	}

	create := exec.CommandContext(ctx, "docker", "volume", "create",
		"--label", fmt.Sprintf("com.docker.compose.project=%s", projectName),
		"--label", fmt.Sprintf("com.docker.compose.volume=%s", volName),
		fullVol,
	)
	create.Stderr = os.Stderr
	if err := create.Run(); err != nil {
		return fmt.Errorf("create volume %s: %w", fullVol, err)
	}

	// Freshly-created volume is empty — just extract. No more `find -delete`
	// dance needed.
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"-u", "0:0",
		"-v", fmt.Sprintf("%s:/dst", fullVol),
		"-v", fmt.Sprintf("%s:/src:ro", inputDirAbs),
		busyboxImage,
		"tar", "-C", "/dst", "-xf", fmt.Sprintf("/src/%s", tarName),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("restore %s: %w", fullVol, err)
	}
	return nil
}

// volumeExists reports whether a named docker volume exists.
func volumeExists(ctx context.Context, fullName string) bool {
	cmd := exec.CommandContext(ctx, "docker", "volume", "inspect", fullName)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}
