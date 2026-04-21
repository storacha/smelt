package snapshot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// projectName resolves the docker-compose project name used to namespace
// volumes. `docker compose` defaults to the lowercased basename of the
// project directory (e.g. "smelt"); callers can override via the
// COMPOSE_PROJECT_NAME env var, which we respect here too.
func projectName(projectDir string) string {
	if v := os.Getenv("COMPOSE_PROJECT_NAME"); v != "" {
		return v
	}
	return strings.ToLower(filepath.Base(projectDir))
}

// composeService mirrors the fields of `docker compose ps --format json` we care about.
type composeService struct {
	Name     string `json:"Name"`
	State    string `json:"State"`
	Health   string `json:"Health"`
	ExitCode int    `json:"ExitCode"`
	// Labels is a raw CSV blob; we parse out compose.oneoff on demand.
	Labels string `json:"Labels"`
}

// stackStatus returns the list of compose-managed services for the project.
// Empty slice + nil error means the stack is fully down.
func stackStatus(ctx context.Context, projectDir string) ([]composeService, error) {
	cmd := exec.CommandContext(ctx, "docker", "compose", "ps", "--all", "--format", "json")
	cmd.Dir = projectDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker compose ps: %w (%s)", err, stderr.String())
	}

	// `docker compose ps --format json` emits NDJSON (one object per line)
	// when there are services, and a JSON array on some compose versions.
	// Handle both.
	raw := bytes.TrimSpace(stdout.Bytes())
	if len(raw) == 0 {
		return nil, nil
	}

	var services []composeService
	if raw[0] == '[' {
		if err := json.Unmarshal(raw, &services); err != nil {
			return nil, fmt.Errorf("parse compose ps json array: %w", err)
		}
		return services, nil
	}

	for _, line := range bytes.Split(raw, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var s composeService
		if err := json.Unmarshal(line, &s); err != nil {
			return nil, fmt.Errorf("parse compose ps ndjson: %w", err)
		}
		services = append(services, s)
	}
	return services, nil
}

// requireStackUp fails if any declared service isn't running-and-healthy.
// Snapshotting a half-up stack would produce an inconsistent checkpoint.
func requireStackUp(ctx context.Context, projectDir string) error {
	services, err := stackStatus(ctx, projectDir)
	if err != nil {
		return err
	}
	if len(services) == 0 {
		return fmt.Errorf("stack is down; run `make up` before saving a snapshot")
	}
	var unhealthy []string
	for _, s := range services {
		// Accept three classes of "OK":
		//   1. Running + healthy (has a healthcheck and it passed).
		//   2. Running + no healthcheck (Health=""), common for email, guppy.
		//   3. Exited + ExitCode=0 (one-shot init containers like ipni-init,
		//      piri-postgres-init that complete and stay exited).
		if s.State == "running" && (s.Health == "" || s.Health == "healthy") {
			continue
		}
		if s.State == "exited" && s.ExitCode == 0 {
			continue
		}
		unhealthy = append(unhealthy, fmt.Sprintf("%s(state=%s,health=%s,exit=%d)",
			s.Name, s.State, s.Health, s.ExitCode))
	}
	if len(unhealthy) > 0 {
		return fmt.Errorf("stack not fully healthy: %s", strings.Join(unhealthy, ", "))
	}
	return nil
}

// requireStackDown fails if any container for this project is still running.
// Restoring volumes while a container holds an open handle corrupts things.
// Uses `docker ps` with a project label filter instead of `docker compose ps`,
// because the latter needs valid compose files — and we may be running right
// after `make nuke` removed them, or about to overwrite smelt.yml with a
// different topology.
func requireStackDown(ctx context.Context, projectDir string) error {
	proj := projectName(projectDir)
	cmd := exec.CommandContext(ctx, "docker", "ps",
		"--filter", fmt.Sprintf("label=com.docker.compose.project=%s", proj),
		"--format", "{{.Names}}",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker ps: %w (%s)", err, stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	var running []string
	for _, l := range lines {
		if l != "" {
			running = append(running, l)
		}
	}
	if len(running) > 0 {
		return fmt.Errorf("stack is still up: %s; run `make down` before loading a snapshot",
			strings.Join(running, ", "))
	}
	return nil
}

// stopStack runs `docker compose stop` so every container receives SIGTERM
// in dependency order. The blockchain container's entrypoint trap turns that
// signal into a `/output/anvil-state.json` dump.
func stopStack(ctx context.Context, projectDir string) error {
	cmd := exec.CommandContext(ctx, "docker", "compose", "stop")
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose stop: %w", err)
	}
	return nil
}

// captureImages resolves the compose config and returns per-service image
// info — both the tag (resolved reference) and the digest (immutable
// content identifier). The digest closes the "same tag, different bytes"
// gap that tag-only capture leaves open: a rolling tag like `:main` resolves
// to different image content depending on when the user last pulled.
func captureImages(ctx context.Context, projectDir string) (map[string]ImageInfo, error) {
	cmd := exec.CommandContext(ctx, "docker", "compose", "config", "--format", "json")
	cmd.Dir = projectDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker compose config: %w (%s)", err, stderr.String())
	}

	// Shape: { "services": { "<name>": { "image": "...", ... }, ... }, ... }
	var doc struct {
		Services map[string]struct {
			Image string `json:"image"`
		} `json:"services"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		return nil, fmt.Errorf("parse compose config: %w", err)
	}

	out := make(map[string]ImageInfo, len(doc.Services))
	for name, svc := range doc.Services {
		if svc.Image == "" {
			continue
		}
		info := ImageInfo{Tag: svc.Image}
		// Best-effort digest lookup. If the image isn't pulled locally
		// (unlikely while the stack is healthy) we record the tag only.
		if digest, err := inspectImageDigest(ctx, svc.Image); err == nil {
			info.Digest = digest
		}
		out[name] = info
	}
	return out, nil
}

// inspectImageDigest returns the canonical immutable identifier for a local
// image. For registry-pulled images this is the RepoDigest
// ("repo@sha256:…"); for locally-built images that have no RepoDigest, the
// docker image Id ("sha256:…").
func inspectImageDigest(ctx context.Context, ref string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", ref,
		"--format", "{{if .RepoDigests}}{{index .RepoDigests 0}}{{else}}{{.Id}}{{end}}")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("inspect %s: %w (%s)", ref, err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

// diffImages returns a sorted slice of human-readable drift descriptions.
// Reports two distinct kinds of drift separately:
//   - tag drift: teammate's `.env` resolves to a different image reference
//   - digest drift at same tag: same reference, different bytes (rolling tag
//     was re-pulled between save and load)
func diffImages(saved, current map[string]ImageInfo) []string {
	names := make(map[string]struct{}, len(saved)+len(current))
	for n := range saved {
		names[n] = struct{}{}
	}
	for n := range current {
		names[n] = struct{}{}
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)

	var out []string
	for _, n := range sorted {
		s, c := saved[n], current[n]
		switch {
		case s.Tag != c.Tag:
			sTag, cTag := s.Tag, c.Tag
			if sTag == "" {
				sTag = "(none)"
			}
			if cTag == "" {
				cTag = "(none)"
			}
			out = append(out, fmt.Sprintf("%s: tag %s → %s", n, sTag, cTag))
		case s.Digest != "" && c.Digest != "" && s.Digest != c.Digest:
			out = append(out, fmt.Sprintf("%s: digest drift at %s (%s → %s)",
				n, s.Tag, shortDigest(s.Digest), shortDigest(c.Digest)))
		}
	}
	return out
}

// shortDigest keeps warning lines compact: a digest like
// "ghcr.io/foo/bar@sha256:abcd1234..." becomes "sha256:abcd1234" (first 16
// hex chars). Unambiguous enough for a warning message.
func shortDigest(d string) string {
	if i := strings.Index(d, "sha256:"); i >= 0 {
		rest := d[i:]
		if len(rest) > 7+16 { // "sha256:" + 16 hex
			return rest[:7+16]
		}
		return rest
	}
	return d
}

// removeStoppedContainers runs `docker compose down` WITHOUT `-v`, removing
// containers (running or stopped) but preserving volumes. Required before
// snapshot load so that `docker volume rm` on each volume isn't blocked by
// stopped containers still holding mounts. Volumes survive compose down
// without -v; our volume restore then wipes and repopulates them.
func removeStoppedContainers(ctx context.Context, projectDir string) error {
	cmd := exec.CommandContext(ctx, "docker", "compose", "down", "--remove-orphans")
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose down: %w", err)
	}
	return nil
}
