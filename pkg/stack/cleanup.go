package stack

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// stackProjectPrefix is the compose project-name prefix every pkg/stack
// stack uses (see sanitizeTestName + NewStack). Containers, volumes, and
// networks created by a stack inherit this as a name prefix, which lets
// the sweeper find them without knowing individual test names.
const stackProjectPrefix = "smeltery-"

// CleanupLeaked removes containers and volumes left behind by prior
// pkg/stack test runs that didn't tear down cleanly (SIGKILL, panic,
// `keepOnFailure` without manual cleanup, oom-killed test binary, etc.).
// Call it from TestMain before running tests to start from a clean slate:
//
//	func TestMain(m *testing.M) {
//	    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
//	    if err := stack.CleanupLeaked(ctx); err != nil {
//	        log.Printf("cleanup warning: %v", err)
//	    }
//	    cancel()
//	    os.Exit(m.Run())
//	}
//
// Only removes resources whose names start with "smeltery-" (the compose
// project prefix used by NewStack). Does not touch the shared
// `storacha-network` since it's reused across test runs, and does not
// interfere with a parallel test suite running against the same host —
// unless that suite ALSO uses pkg/stack and happens to be live when
// CleanupLeaked runs. For single-suite test machines this is safe.
func CleanupLeaked(ctx context.Context) error {
	if err := removeByPrefix(ctx, leakedContainers, stackProjectPrefix); err != nil {
		return fmt.Errorf("cleanup containers: %w", err)
	}
	if err := removeByPrefix(ctx, leakedVolumes, stackProjectPrefix); err != nil {
		return fmt.Errorf("cleanup volumes: %w", err)
	}
	return nil
}

type listRemover struct {
	list   func(ctx context.Context, namePrefix string) ([]string, error)
	remove func(ctx context.Context, ids []string) error
}

var leakedContainers = listRemover{
	list: func(ctx context.Context, namePrefix string) ([]string, error) {
		out, err := exec.CommandContext(ctx, "docker", "ps", "-a",
			"--filter", "name="+namePrefix,
			"--format", "{{.ID}}").Output()
		if err != nil {
			return nil, err
		}
		return splitLines(string(out)), nil
	},
	remove: func(ctx context.Context, ids []string) error {
		args := append([]string{"rm", "-f"}, ids...)
		return exec.CommandContext(ctx, "docker", args...).Run()
	},
}

var leakedVolumes = listRemover{
	list: func(ctx context.Context, namePrefix string) ([]string, error) {
		out, err := exec.CommandContext(ctx, "docker", "volume", "ls",
			"--filter", "name="+namePrefix,
			"--format", "{{.Name}}").Output()
		if err != nil {
			return nil, err
		}
		return splitLines(string(out)), nil
	},
	remove: func(ctx context.Context, names []string) error {
		args := append([]string{"volume", "rm", "-f"}, names...)
		return exec.CommandContext(ctx, "docker", args...).Run()
	},
}

func removeByPrefix(ctx context.Context, r listRemover, prefix string) error {
	ids, err := r.list(ctx, prefix)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	return r.remove(ctx, ids)
}

func splitLines(s string) []string {
	var out []string
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			out = append(out, line)
		}
	}
	return out
}
