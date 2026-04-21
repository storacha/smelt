package stack

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

// hostPortLine matches a compose ports-list entry of the form
// `- "HOST:CONTAINER"` OR `- HOST:CONTAINER` (optionally followed by a yaml
// comment). Both forms appear in the extracted tree: the static compose
// files quote their port mappings by hand, while yaml.v3 emits the generated
// piri.yml with unquoted numeric-colon-numeric strings. The opening and
// closing quotes are captured as a pair so whichever form the line had going
// in, it keeps going out.
//
// The match is deliberately narrow: anything else inside the quotes (env
// vars, multiaddrs, three-field port specs with an explicit interface) is
// left alone.
var hostPortLine = regexp.MustCompile(`(?m)^([ \t]+- )("?)(\d+):(\d+)("?)([ \t].*)?$`)

// rewriteExtractedForEphemeralPorts edits the extracted compose files in-place
// so every host-side port binding becomes ephemeral. Two stacks running in the
// same test binary (or across parallel binaries) then get disjoint random
// ports from Docker instead of colliding on fixed numbers.
//
// It also rewrites the upload service's config.yaml so sprue's public_url is
// the in-network hostname rather than a now-random host:port. The validation
// emails sprue mints embed that URL; see pkg/clients/guppy for the in-network
// clicker that POSTs back to it.
//
// Finally, it drops `external: true` from the root compose's storacha-network
// declaration so compose provisions a project-scoped network per stack.
// Parallel stacks would otherwise all attach to the same shared bridge and
// collide on service DNS names (two `piri-0`s, two `upload`s, etc.).
//
// Must be called after [extractFiles] and after the generator writes
// `generated/compose/piri.yml` — both live under tempDir.
func rewriteExtractedForEphemeralPorts(tempDir string) error {
	if err := stripHostPortsInDir(tempDir); err != nil {
		return fmt.Errorf("strip host ports: %w", err)
	}
	if err := rewriteSprueConfig(tempDir); err != nil {
		return fmt.Errorf("rewrite sprue config: %w", err)
	}
	if err := rewriteRootNetwork(tempDir); err != nil {
		return fmt.Errorf("rewrite root network: %w", err)
	}
	return nil
}

// externalNetworkBlock matches the root compose's external-network declaration
// and the (optional) blank line that follows it. It's anchored to the exact
// shape we ship so accidental edits elsewhere in the file don't trigger.
var externalNetworkBlock = regexp.MustCompile(`(?m)^networks:\s*\n\s+storacha-network:\s*\n\s+external:\s*true\s*\n`)

// rewriteRootNetwork replaces the root compose's external storacha-network
// declaration with an unqualified one, so compose creates a project-scoped
// network for each stack. Individual system compose files only *reference*
// `storacha-network`; they are untouched.
func rewriteRootNetwork(tempDir string) error {
	path := filepath.Join(tempDir, "compose.yml")
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	rewritten := externalNetworkBlock.ReplaceAll(content, []byte("networks:\n  storacha-network: {}\n"))
	if bytes.Equal(rewritten, content) {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, rewritten, info.Mode())
}

// stripHostPortsInDir walks tempDir and, for every `compose.yml` (or the
// generated `piri.yml`), rewrites each `- "HOST:CONTAINER"` ports entry into
// `- "CONTAINER"`. The container-side port is preserved verbatim, so Docker
// still exposes the right port inside the network; only the host binding
// becomes ephemeral.
func stripHostPortsInDir(tempDir string) error {
	return filepath.WalkDir(tempDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		base := filepath.Base(path)
		if base != "compose.yml" && base != "piri.yml" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rewritten := hostPortLine.ReplaceAll(content, []byte("${1}${2}${4}${5}${6}"))
		if bytes.Equal(rewritten, content) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(path, rewritten, info.Mode())
	})
}

// rewriteSprueConfig flips public_url in the extracted upload config to the
// in-network hostname. Sprue bakes public_url into validation emails; with
// ephemeral host ports there is no stable host:port to point at, so we point
// at Docker's internal DNS name instead and click the link from inside a
// container.
func rewriteSprueConfig(tempDir string) error {
	path := filepath.Join(tempDir, "systems", "upload", "config", "config.yaml")
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	re := regexp.MustCompile(`(?m)^(\s*public_url:\s*).*$`)
	rewritten := re.ReplaceAll(content, []byte("${1}http://upload:80"))
	if bytes.Equal(rewritten, content) {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, rewritten, info.Mode())
}
