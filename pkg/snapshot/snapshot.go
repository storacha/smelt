// Package snapshot captures and restores the full state of a running smelt
// stack so subsequent boots can skip the multi-minute on-chain registration
// dance. A snapshot includes:
//
//   - The anvil chain state dumped by the blockchain container on shutdown
//   - Every docker-compose volume referenced by the resolved manifest
//   - All service identity keys under generated/keys/
//   - A copy of smelt.yml for provenance
//
// Snapshots live under generated/snapshots/<name>/ and are not intended to
// travel between machines — paths, project names, and image tags are local.
package snapshot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/storacha/smelt/pkg/generate"
	"github.com/storacha/smelt/pkg/manifest"
)

// Fixed subdirectories written inside every snapshot.
const (
	subdirBlockchain = "blockchain"
	subdirKeys       = "keys"
	subdirProofs     = "proofs"
	subdirVolumes    = "volumes"
	manifestCopy     = "smelt.yml"
)

// Paths within the project layout the snapshot code reads from or writes to.
const (
	projKeysDir      = "generated/keys"
	projProofsDir    = "generated/proofs"
	projSnapshotsDir = "generated/snapshots"
	// Scratch is both the destination for saved snapshots' blockchain state
	// (so the next `make up` reads from it via the compose bind-mount) and
	// the sink for the blockchain container's SIGTERM dump on every stop.
	projScratchDir = "generated/snapshot-scratch"
)

// SaveOpts drives snapshot creation.
type SaveOpts struct {
	ProjectDir string
	Name       string
	Force      bool // overwrite an existing snapshot with the same name
}

// Save captures the current stack state under generated/snapshots/<name>/.
// The stack is left stopped on success so the user can decide whether to
// restart it or immediately work from the newly-saved baseline.
func Save(ctx context.Context, opts SaveOpts) error {
	if err := validateName(opts.Name); err != nil {
		return err
	}
	projectDir, err := filepath.Abs(opts.ProjectDir)
	if err != nil {
		return fmt.Errorf("resolve project dir: %w", err)
	}

	// Use the same resolver as Generate — a save during a snapshot session
	// captures the session's manifest, keeping the saved snapshot consistent
	// with the running stack.
	manifestPath, _ := manifest.ResolveManifestPath(projectDir)
	m, err := manifest.Parse(manifestPath)
	if err != nil {
		return err
	}
	vols, err := resolveVolumes(m)
	if err != nil {
		return err
	}

	snapsRoot := filepath.Join(projectDir, projSnapshotsDir)
	finalDir := filepath.Join(snapsRoot, opts.Name)
	stagingDir := filepath.Join(snapsRoot, "."+opts.Name+".tmp")

	if _, err := os.Stat(finalDir); err == nil {
		if !opts.Force {
			return fmt.Errorf("snapshot %q already exists; use --force to overwrite", opts.Name)
		}
	}

	if err := os.MkdirAll(snapsRoot, 0755); err != nil {
		return fmt.Errorf("create snapshots dir: %w", err)
	}
	if err := os.RemoveAll(stagingDir); err != nil {
		return fmt.Errorf("clear staging dir: %w", err)
	}
	if err := os.MkdirAll(stagingDir, 0755); err != nil {
		return fmt.Errorf("create staging dir: %w", err)
	}

	// On any failure past this point, remove the staging dir so the user
	// doesn't accumulate half-baked scratch between attempts.
	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(stagingDir)
		}
	}()

	fmt.Printf("Checking stack health...\n")
	if err := requireStackUp(ctx, projectDir); err != nil {
		return err
	}

	// Capture image tags before stopping — once the stack is down,
	// `docker compose config` still works but we want to snapshot the
	// state that was running, not re-resolve against whatever env the
	// user tinkers with between stop and save.
	images, err := captureImages(ctx, projectDir)
	if err != nil {
		return fmt.Errorf("capture images: %w", err)
	}

	// Clear the scratch dir so we know any files that appear came from THIS
	// stop, not a prior run. The blockchain container's trap writes atomically
	// (.tmp + mv) so a crash mid-write can't confuse us either way.
	scratchDir := filepath.Join(projectDir, projScratchDir)
	if err := clearDir(scratchDir); err != nil {
		return fmt.Errorf("clear scratch dir: %w", err)
	}

	fmt.Printf("Stopping stack (triggers blockchain state dump)...\n")
	if err := stopStack(ctx, projectDir); err != nil {
		return err
	}

	// Wait briefly for the scratch files to land — compose stop returns once
	// containers have exited, but the trap's `mv` may trail by a few ms.
	if err := waitForFile(filepath.Join(scratchDir, "anvil-state.json"), 5*time.Second); err != nil {
		return fmt.Errorf("blockchain did not produce an anvil-state.json on shutdown: %w", err)
	}

	fmt.Printf("Archiving blockchain state...\n")
	bcStaging := filepath.Join(stagingDir, subdirBlockchain)
	if err := os.MkdirAll(bcStaging, 0755); err != nil {
		return err
	}
	for _, f := range []string{"anvil-state.json", "deployed-addresses.json"} {
		src := filepath.Join(scratchDir, f)
		dst := filepath.Join(bcStaging, f)
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("copy %s: %w", f, err)
		}
	}

	fmt.Printf("Archiving keys...\n")
	keysSrc := filepath.Join(projectDir, projKeysDir)
	keysDst := filepath.Join(stagingDir, subdirKeys)
	keyFiles, err := copyDir(keysSrc, keysDst)
	if err != nil {
		return fmt.Errorf("archive keys: %w", err)
	}

	fmt.Printf("Archiving proofs...\n")
	proofsSrc := filepath.Join(projectDir, projProofsDir)
	proofsDst := filepath.Join(stagingDir, subdirProofs)
	proofFiles, err := copyDir(proofsSrc, proofsDst)
	if err != nil {
		return fmt.Errorf("archive proofs: %w", err)
	}

	fmt.Printf("Archiving %d volume(s)...\n", len(vols))
	volsDst := filepath.Join(stagingDir, subdirVolumes)
	if err := os.MkdirAll(volsDst, 0755); err != nil {
		return err
	}
	proj := projectName(projectDir)
	for _, v := range vols {
		fmt.Printf("  %s\n", v)
		if err := archiveVolume(ctx, proj, v, volsDst); err != nil {
			return err
		}
	}

	fmt.Printf("Copying %s (provenance)...\n", filepath.Base(manifestPath))
	if err := copyFile(
		manifestPath,
		filepath.Join(stagingDir, manifestCopy),
	); err != nil {
		return err
	}

	if err := writeDescriptor(stagingDir, &Descriptor{
		Name:      opts.Name,
		CreatedAt: time.Now().UTC(),
		Volumes:   vols,
		Keys:      keyFiles,
		Proofs:    proofFiles,
		Images:    images,
	}); err != nil {
		return err
	}

	// Atomic commit: rename staging → final. On an overwrite we remove the
	// old final AFTER staging is complete, so a partial failure leaves the
	// previous snapshot intact.
	if opts.Force {
		if err := os.RemoveAll(finalDir); err != nil {
			return fmt.Errorf("remove old snapshot: %w", err)
		}
	}
	if err := os.Rename(stagingDir, finalDir); err != nil {
		return fmt.Errorf("commit snapshot: %w", err)
	}
	success = true

	fmt.Printf("\nSaved snapshot %q → %s\n", opts.Name, finalDir)
	fmt.Printf("Stack is stopped. Run `make up` to restart.\n")
	return nil
}

// LoadOpts drives snapshot restoration.
type LoadOpts struct {
	ProjectDir string
	// NameOrPath is either a snapshot name (resolved under
	// generated/snapshots/<name>/) or a path (absolute, or containing "/")
	// pointing directly at a snapshot directory.
	NameOrPath string
}

// LoadFilesPaths targets the filesystem-restore portion of a snapshot.
// Any field left empty is skipped. Used by Load (compose path, fills all
// four) and pkg/stack (Go test stack, typically omits SessionManifestPath).
type LoadFilesPaths struct {
	// KeysDir receives the snapshot's keys/*. Cleared before copy.
	KeysDir string
	// ProofsDir receives the snapshot's proofs/*. Cleared before copy.
	// Tolerated: snapshots without a proofs/ subdir (older format).
	ProofsDir string
	// ScratchDir receives the snapshot's blockchain/{anvil-state,
	// deployed-addresses}.json pair. Compose expects to find them here
	// via its bind-mount.
	ScratchDir string
	// SessionManifestPath receives a copy of the snapshot's smelt.yml.
	// Set to the session-manifest path for the compose flow; leave empty
	// for the Go SDK (tests have no session semantics).
	SessionManifestPath string
}

// LoadFiles copies the filesystem portion of a snapshot into the given
// destination paths. Returns the snapshot's descriptor so callers can
// restore docker volumes separately (via RestoreVolume). Does not touch
// any docker resources, does not regenerate compose files, does not
// manage the running stack — those concerns belong to the caller.
func LoadFiles(ctx context.Context, snapshotDir string, dst LoadFilesPaths) (*Descriptor, error) {
	desc, err := readDescriptor(snapshotDir)
	if err != nil {
		return nil, err
	}

	if dst.SessionManifestPath != "" {
		if err := os.MkdirAll(filepath.Dir(dst.SessionManifestPath), 0755); err != nil {
			return nil, fmt.Errorf("create session manifest dir: %w", err)
		}
		if err := copyFile(
			filepath.Join(snapshotDir, manifestCopy),
			dst.SessionManifestPath,
		); err != nil {
			return nil, fmt.Errorf("install session manifest: %w", err)
		}
	}

	if dst.ScratchDir != "" {
		if err := os.MkdirAll(dst.ScratchDir, 0755); err != nil {
			return nil, err
		}
		for _, f := range []string{"anvil-state.json", "deployed-addresses.json"} {
			if err := copyFile(
				filepath.Join(snapshotDir, subdirBlockchain, f),
				filepath.Join(dst.ScratchDir, f),
			); err != nil {
				return nil, fmt.Errorf("restore %s: %w", f, err)
			}
		}
	}

	if dst.KeysDir != "" {
		if err := os.RemoveAll(dst.KeysDir); err != nil {
			return nil, fmt.Errorf("clear keys dir: %w", err)
		}
		if err := os.MkdirAll(dst.KeysDir, 0755); err != nil {
			return nil, err
		}
		if _, err := copyDir(filepath.Join(snapshotDir, subdirKeys), dst.KeysDir); err != nil {
			return nil, fmt.Errorf("restore keys: %w", err)
		}
	}

	if dst.ProofsDir != "" {
		if err := os.RemoveAll(dst.ProofsDir); err != nil {
			return nil, fmt.Errorf("clear proofs dir: %w", err)
		}
		if err := os.MkdirAll(dst.ProofsDir, 0755); err != nil {
			return nil, err
		}
		// Older snapshots (pre-proofs) have no proofs subdir; that's fine.
		if snapProofs := filepath.Join(snapshotDir, subdirProofs); dirExists(snapProofs) {
			if _, err := copyDir(snapProofs, dst.ProofsDir); err != nil {
				return nil, fmt.Errorf("restore proofs: %w", err)
			}
		}
	}

	return desc, nil
}

// Load restores the named snapshot over the current project state. The stack
// must be fully stopped. Keys and proofs are overwritten (gitignored),
// the blockchain state file is written to the scratch dir, the snapshot's
// smelt.yml becomes the session manifest at generated/snapshot-scratch/smelt.yml
// (project root smelt.yml is never touched), and each archived volume is
// extracted into its named docker volume.
func Load(ctx context.Context, opts LoadOpts) error {
	if opts.NameOrPath == "" {
		return errors.New("snapshot name or path is required")
	}
	projectDir, err := filepath.Abs(opts.ProjectDir)
	if err != nil {
		return fmt.Errorf("resolve project dir: %w", err)
	}

	snapDir, err := resolveSnapshotDir(projectDir, opts.NameOrPath)
	if err != nil {
		return err
	}

	// Check for running containers BEFORE touching anything — we need to
	// confirm the stack is down against whatever topology is currently
	// resolved, not the topology we're about to install.
	fmt.Printf("Checking stack is down...\n")
	if err := requireStackDown(ctx, projectDir); err != nil {
		return err
	}

	// Restore the filesystem portion of the snapshot: session manifest,
	// blockchain state files, keys, proofs. This matches what the compose
	// layer expects via its bind-mounts.
	fmt.Printf("Restoring files (session manifest, blockchain state, keys, proofs)...\n")
	desc, err := LoadFiles(ctx, snapDir, LoadFilesPaths{
		KeysDir:             filepath.Join(projectDir, projKeysDir),
		ProofsDir:           filepath.Join(projectDir, projProofsDir),
		ScratchDir:          filepath.Join(projectDir, projScratchDir),
		SessionManifestPath: filepath.Join(projectDir, manifest.SessionManifestPath),
	})
	if err != nil {
		return err
	}

	// Regenerate compose files so they match the freshly-installed session
	// manifest. A fresh `make nuke` removes generated/compose/piri.yml;
	// without it, `docker compose down` below fails parsing the compose
	// include.
	if _, err := generate.Generate(generate.Options{
		ProjectDir: projectDir,
	}); err != nil {
		return fmt.Errorf("regenerate compose files: %w", err)
	}

	// `docker volume rm` refuses for volumes attached to stopped-but-extant
	// containers. `make down` may leave those around depending on flags, so
	// normalise the slate here.
	fmt.Printf("Removing any stopped containers...\n")
	if err := removeStoppedContainers(ctx, projectDir); err != nil {
		return err
	}

	// Warn if images differ between what the snapshot was taken against and
	// what the current compose config would pull. Covers two distinct cases:
	//   - Tag drift: teammate's .env has different image references.
	//   - Digest drift at same tag: rolling tags like :main were re-pulled
	//     between save and load, so the bytes differ even though the
	//     reference looks identical.
	if len(desc.Images) > 0 {
		currentImages, err := captureImages(ctx, projectDir)
		if err != nil {
			// Non-fatal — snapshot contents already landed; if compose config
			// can't resolve now, make up will fail loudly anyway.
			fmt.Fprintf(os.Stderr, "warning: could not resolve current images for drift check: %v\n", err)
		} else if drift := diffImages(desc.Images, currentImages); len(drift) > 0 {
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "WARNING: images differ from snapshot:")
			for _, line := range drift {
				fmt.Fprintf(os.Stderr, "  %s\n", line)
			}
			fmt.Fprintln(os.Stderr, "  The snapshot's state was produced by the 'saved' images;")
			fmt.Fprintln(os.Stderr, "  behavior may differ if the 'current' images don't match.")
			fmt.Fprintln(os.Stderr, "")
		}
	}

	fmt.Printf("Restoring %d volume(s)...\n", len(desc.Volumes))
	proj := projectName(projectDir)
	volsSrc := filepath.Join(snapDir, subdirVolumes)
	for _, v := range desc.Volumes {
		fmt.Printf("  %s\n", v)
		if err := RestoreVolume(ctx, proj, v, volsSrc); err != nil {
			return err
		}
	}

	fmt.Printf("\nLoaded snapshot from %s.\n", snapDir)
	fmt.Printf("Run `make up` to start the stack from this state.\n")
	return nil
}

// Info is a summary row for snapshot listings.
type Info struct {
	Name      string
	CreatedAt time.Time
	Volumes   []string
	SizeBytes int64
}

// List returns all snapshots under the project.
func List(projectDir string) ([]Info, error) {
	root := filepath.Join(projectDir, projSnapshotsDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []Info
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dir := filepath.Join(root, e.Name())
		desc, err := readDescriptor(dir)
		if err != nil {
			// Unreadable descriptor is suspicious but not fatal — show the
			// directory with unknown metadata so the user can investigate.
			out = append(out, Info{Name: e.Name()})
			continue
		}
		size, _ := dirSize(dir)
		out = append(out, Info{
			Name:      desc.Name,
			CreatedAt: desc.CreatedAt,
			Volumes:   desc.Volumes,
			SizeBytes: size,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// Remove deletes the named snapshot directory.
func Remove(projectDir, name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	dir := filepath.Join(projectDir, projSnapshotsDir, name)
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("snapshot %q not found", name)
		}
		return err
	}
	return os.RemoveAll(dir)
}

// validateName rejects path-traversal and empty names.
func validateName(name string) error {
	if name == "" {
		return errors.New("snapshot name is required")
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return fmt.Errorf("invalid snapshot name %q", name)
	}
	return nil
}

// resolveSnapshotDir accepts either a snapshot name (validated and looked up
// under generated/snapshots/) or a path (absolute, or containing "/" — e.g.
// relative path outside the project). Returns the resolved absolute path to
// the snapshot directory.
func resolveSnapshotDir(projectDir, nameOrPath string) (string, error) {
	if nameOrPath == "" {
		return "", errors.New("snapshot name or path is required")
	}
	var candidate string
	if filepath.IsAbs(nameOrPath) || strings.ContainsAny(nameOrPath, `/\`) {
		// Treat as path. Resolve relative paths against the current working
		// directory (user-intuitive; matches how shells interpret the arg).
		abs, err := filepath.Abs(nameOrPath)
		if err != nil {
			return "", fmt.Errorf("resolve snapshot path: %w", err)
		}
		candidate = abs
	} else {
		if err := validateName(nameOrPath); err != nil {
			return "", err
		}
		candidate = filepath.Join(projectDir, projSnapshotsDir, nameOrPath)
	}
	if _, err := os.Stat(candidate); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("snapshot not found at %s", candidate)
		}
		return "", err
	}
	// Sanity: the directory should contain a descriptor.
	if _, err := os.Stat(filepath.Join(candidate, DescriptorFile)); err != nil {
		return "", fmt.Errorf("not a valid snapshot dir (missing %s): %s", DescriptorFile, candidate)
	}
	return candidate, nil
}

// clearDir removes directory contents without removing the directory itself.
func clearDir(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// copyFile copies a regular file, preserving mode. Unlinks the destination
// first so we can overwrite files we don't own (e.g. scratch state files
// written by the blockchain container as root). Unlinking only requires
// write+execute on the parent directory, which smelt always has.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove existing %s: %w", dst, err)
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// copyDir copies the contents of src into dst non-recursively for the top
// level, recursively via filepath.WalkDir. Returns the list of relative paths
// copied (used for descriptor bookkeeping).
func copyDir(src, dst string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0755)
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		if err := copyFile(path, target); err != nil {
			return err
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// dirSize sums the size of all regular files under dir.
func dirSize(dir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	return total, err
}

// dirExists reports whether path exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// waitForFile polls for a file's existence up to the given timeout. Used to
// bridge the small gap between `docker compose stop` returning and the
// entrypoint trap's final `mv` landing.
func waitForFile(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for %s", path)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
