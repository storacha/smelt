# Snapshots

Smelt can capture the complete state of a running stack to disk and later
restore it — contract state on chain, docker volumes, identity keys, UCAN
proofs, and the manifest that described the topology. A snapshot-restored
boot reaches "piri registered, ready to upload" in ~10s instead of the
~45s a cold boot costs.

## Objective

A cold boot of smelt is dominated by on-chain activity: contract
deployment, piri registration with the delegator and upload services,
and PDP data-set creation. Each of those steps waits on blockchain
finality (several epochs × 3s), serializes across multiple piri nodes,
and totals tens of seconds to multiple minutes depending on topology.
For iterative development — tweak code, restart stack, reproduce a bug —
paying this cost on every cycle is brutal.

Snapshots let you pay the cost once, then resume from the checkpointed
state on every subsequent boot.

## When to use

- You brought up a stack, it reached a known-good state, and you want
  to preserve that state for tomorrow's dev session.
- You're iterating on a service and want to skip the multi-minute
  registration dance on every restart.
- You want to switch between different scenarios (1 piri vs 3, sqlite
  vs postgres) without paying full cold-boot costs each time.

## Prerequisites

- **Docker engine 25+**. Smelt pins the compose project name and uses
  `healthcheck.start_interval` (both added in engine 25). The Makefile's
  `check-docker` target fails early with an upgrade pointer if you're
  on an older engine.
- **Linux or macOS host**. Smelt assumes a Unix-family docker host.

## When NOT to use

- **CI.** CI should exercise the cold-boot path — that's what's broken
  if an upstream image or contract regresses. Snapshots hide regressions.
- **Contract code changes.** The committed baseline
  (`systems/blockchain/state/anvil-state.json`) encodes deployed contract
  addresses and state. Saved snapshots encode them too. When
  filecoin-services ships a new release, recapture your snapshots.

## Quick start

Stack up and healthy:

```bash
./smelt snapshot save baseline
```

Later, restore and boot in one step:

```bash
YES=1 make nuke
make up SNAPSHOT=baseline
```

The stack comes up in ~10s, already at the state you saved.

## Commands

### `./smelt snapshot save <name>`

Captures the running stack into `generated/snapshots/<name>/`. Stops the
stack gracefully (so the blockchain can dump chain state on SIGTERM),
archives each docker volume, copies keys/proofs/manifest, writes a
descriptor. The stack is left stopped on success — run `make up` to
resume or `make up SNAPSHOT=<name>` to restart from the saved point.

The stack must be fully healthy at save time. A save of a half-healthy
stack would produce an inconsistent checkpoint.

### `./smelt snapshot list`

Table of known snapshots with age, size, and volume count.

### `./smelt snapshot rm <name>`

Deletes the snapshot directory. No undo.

### `make up SNAPSHOT=<name-or-path>`

The canonical way to boot from a snapshot. Accepts either a name
(resolved under `generated/snapshots/<name>/`) or a path (absolute or
relative, for snapshots elsewhere on disk):

```bash
make up SNAPSHOT=baseline
make up SNAPSHOT=/tmp/archived-snap
```

Under the hood, this runs `./smelt snapshot load <value>` and then the
normal `make up` flow.

### `./smelt snapshot load <name-or-path>`

A building block for `make up SNAPSHOT=…`. Populates on-disk state from
the snapshot but doesn't start the stack. Useful for tests or when you
want to poke at the restored state before boot.

## What a snapshot contains

```
generated/snapshots/<name>/
├── manifest.json                # name, created_at, volumes, keys, proofs, images{tag,digest}
├── smelt.yml                    # topology at save time (session manifest source)
├── blockchain/
│   ├── anvil-state.json         # chain state captured via SIGTERM dump
│   └── deployed-addresses.json  # PDP contract addresses
├── keys/                        # every *.pem, *.pub, *.hex in generated/keys/
├── proofs/                      # every *.txt in generated/proofs/
└── volumes/                     # .tar per named docker volume
    ├── piri-0-data.tar
    ├── dynamodb-data.tar        # delegator allow list, upload registry
    ├── minio-data.tar           # upload's S3 backend
    ├── ipni-data.tar            # content discovery index
    ├── guppy-data.tar           # client login + spaces
    ├── piri-postgres-data.tar   # only when topology uses postgres
    └── piri-minio-data.tar      # only when topology uses S3
```

Tracked files at the project root (`smelt.yml`,
`systems/blockchain/state/*.json`) are never modified by a load. Your
git working tree stays clean.

## The session manifest

When you run `make up SNAPSHOT=X`, smelt copies the snapshot's
`smelt.yml` to `generated/snapshot-scratch/smelt.yml`. This path is
gitignored and acts as the **session manifest**: while it exists,
`smelt generate` and `smelt snapshot save` read from it instead of the
project's tracked `smelt.yml`.

This has two important consequences:

1. **Subsequent `make up` (without SNAPSHOT) stays on the same
   topology.** The session persists across `make down` / `make up`
   cycles until you explicitly end it. This is what makes the
   resume-from-dump flow work: you can boot from a snapshot, stop,
   come back tomorrow, and continue.

2. **Your tracked `smelt.yml` is never silently overridden.** Edits
   to it don't take effect while a session is active. To apply changes
   from `smelt.yml`, end the session first.

Ending a session:

- `make clean` — wipes volumes + chain state + session manifest.
- `make nuke` / `make fresh` — same, plus keys/proofs.

After either, the next `make up` reads the project's `smelt.yml`.

## Workflows

### Preserve a known-good stack

```bash
make up                             # cold boot; wait for healthy
./smelt snapshot save good
```

### Resume tomorrow

```bash
make up SNAPSHOT=good
# ... work ...
make down                           # pause; volumes + chain dump preserved
make up                             # resume exactly where you left off
```

The second `make up` reads the session manifest, so there's no
SNAPSHOT= needed — you're still in the session.

### Switch topologies

```bash
# Save a sqlite+filesystem baseline
./smelt snapshot save sqlite-baseline

# Edit smelt.yml (topology becomes postgres+s3 with 3 piri)
YES=1 make clean                    # wipe state and end any prior session
make up                             # cold boot new topology
./smelt snapshot save postgres-s3-3x

# Later, switch freely
YES=1 make clean
make up SNAPSHOT=sqlite-baseline

YES=1 make clean
make up SNAPSHOT=postgres-s3-3x
```

### Archive a snapshot outside the project

```bash
cp -r generated/snapshots/good /tmp/good-snap
# Later, possibly from a different clone of smelt:
make up SNAPSHOT=/tmp/good-snap
```

## Gotchas

### Postgres version changes

A postgres data directory is version-specific. If `systems/piri/` is
regenerated against a bumped `postgres:17` image, a snapshot saved
with `postgres:16` will fail to boot — postgres refuses to run older
on-disk format against a newer server. Recapture the snapshot after
version bumps.

### Contract code changes

An upstream filecoin-services release changes the committed baseline
`anvil-state.json`. Your existing snapshots are still on the old
contracts and may behave unexpectedly against a mixed-version stack.
Treat filecoin-services releases as "recapture snapshots."

### Sharing snapshots across teammates

Snapshots are portable across Linux/macOS checkouts as long as:

- The compose project name is pinned (`name: smelt` at the top of
  `compose.yml` does this — shipped by default).
- Docker engine ≥ 25 on both saver and loader (enforced by Make's
  `check-docker` target; see Prerequisites).
- Image identity matches between save and load.

Smelt captures **both the image reference (tag) and the content digest**
for every service at save time. On load it compares against the current
compose config and reports two kinds of drift independently:

```
WARNING: images differ from snapshot:
  piri-0: tag ghcr.io/storacha/piri:main → ghcr.io/storacha/piri:test-build
  blockchain: digest drift at filecoin-localdev:local (sha256:83bfc639… → sha256:a1b2c3d4…)
```

- **Tag drift** means your `.env` resolves a service to a different image
  reference than the snapshot was saved against — likely a local override.
- **Digest drift at the same tag** catches the rolling-tag silent-pull
  case: both sides use `ghcr.io/storacha/piri:main`, but one pulled
  Monday and one pulled Wednesday, so the bytes differ.

A warning doesn't block the restore. It's a heads-up: if the image
actually changed, behavior may diverge from what the snapshot's state
was produced against.

Committing snapshots: smelt expects committed, team-shared snapshots
to live under `snapshots/` at the project root (not gitignored), while
personal/throwaway ones stay in `generated/snapshots/` (gitignored by
the existing `generated/` rule). `make up SNAPSHOT=…` accepts either
a name (resolved under `generated/snapshots/`) or a path (use for
committed ones, e.g. `SNAPSHOT=snapshots/quickstart`).

**Windows / non-Unix hosts**: not supported. Smelt assumes a Linux or
macOS docker host.

### Snapshots aren't free (~2–5 MiB each)

Volume tarballs compress the data dirs but still add up. `./smelt
snapshot list` shows sizes; `rm` as you go.

### Topology in snapshot vs your smelt.yml

The snapshot's `smelt.yml` is authoritative while a session is active —
your tracked `smelt.yml` edits don't apply. Run `make clean` first to
leave the session and pick up your changes.

## Troubleshooting

### "stack is still up" on load

Run `make down` first. A snapshot load must wipe docker volumes, and
docker refuses to remove volumes attached to running containers.

### "volume is in use" on load

Stopped-but-extant containers are still holding mounts. Load runs
`docker compose down --remove-orphans` automatically to clean these
up; if you still see it, manually run that command and retry.

### Blockchain unhealthy after a load

Most likely, the snapshot's chain state has drifted from the
filecoin-services image referenced by your current `.env`. The load
output prints a `WARNING: image tags differ from snapshot:` block for
exactly this case — check which services drifted and either match
their tags to what the snapshot expects, or recapture the snapshot
against your current images.

### `git status` shows `smelt.yml` modified after operations

It shouldn't — `snapshot load` writes to `generated/snapshot-scratch/`
only. If you see it, you may be on an older binary; rebuild:

```bash
go build -o smelt ./cmd/smelt
```

### I edited smelt.yml but `make up` didn't pick up the change

You're in a snapshot session. Run `make clean` to end it and re-run
`make up`. Confirm via `ls generated/snapshot-scratch/smelt.yml`
(should be gone).
