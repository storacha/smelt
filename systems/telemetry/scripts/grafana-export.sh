#!/usr/bin/env bash
# grafana-export.sh - Dump the live Grafana's dashboards and unified-alerting
# configuration into the on-disk provisioning tree so it can be committed.
#
# Usage: systems/telemetry/scripts/grafana-export.sh
#   GRAFANA_URL=http://localhost:15200   (default)
#   OUT_DIR=systems/telemetry/config/grafana   (default, resolved relative to repo root)
#
# Requires the telemetry stack to be running (make up-telemetry). Grafana has
# anonymous admin enabled in this environment so no auth headers are needed.

set -euo pipefail

GRAFANA_URL="${GRAFANA_URL:-http://localhost:15200}"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
OUT_DIR="${OUT_DIR:-$REPO_ROOT/systems/telemetry/config/grafana}"
DASH_DIR="$OUT_DIR/dashboards"
ALERT_DIR="$OUT_DIR/alerting"

# Sanity checks
for bin in curl jq; do
    if ! command -v "$bin" >/dev/null 2>&1; then
        echo "ERROR: '$bin' is required but not installed" >&2
        exit 1
    fi
done

if ! curl -fsS "$GRAFANA_URL/api/health" >/dev/null 2>&1; then
    echo "ERROR: Grafana is not reachable at $GRAFANA_URL" >&2
    echo "       Run 'make up-telemetry' to start the telemetry stack." >&2
    exit 1
fi

mkdir -p "$DASH_DIR" "$ALERT_DIR"

slugify() {
    # Lowercase, replace non-alphanumeric runs with '-', strip leading/trailing '-'.
    echo "$1" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9]+/-/g; s/^-+//; s/-+$//'
}

echo "=> Exporting dashboards from $GRAFANA_URL"

# List all dashboards (type=dash-db filters out folders).
mapfile -t DASH_ROWS < <(
    curl -fsS "$GRAFANA_URL/api/search?type=dash-db" \
        | jq -c '.[] | {uid, title, folder: (.folderTitle // "overview")}'
)

dashboard_count=0
for row in "${DASH_ROWS[@]}"; do
    uid=$(echo "$row" | jq -r '.uid')
    title=$(echo "$row" | jq -r '.title')
    folder=$(echo "$row" | jq -r '.folder')

    folder_slug=$(slugify "$folder")
    file_slug=$(slugify "$title")

    target_dir="$DASH_DIR/$folder_slug"
    mkdir -p "$target_dir"
    target_file="$target_dir/$file_slug.json"

    # Fetch the full dashboard. Strip .id and .version so re-imports don't
    # conflict; keep .uid so future exports overwrite the same file.
    curl -fsS "$GRAFANA_URL/api/dashboards/uid/$uid" \
        | jq '.dashboard | del(.id, .version)' \
        > "$target_file"

    echo "   $folder_slug/$file_slug.json  ($title)"
    dashboard_count=$((dashboard_count + 1))
done

echo "   exported $dashboard_count dashboard(s)"

echo "=> Exporting unified alerting config"

export_alerting() {
    local path=$1
    local outfile=$2
    local label=$3

    # `export?format=yaml` returns provisioning-ready YAML. On fresh stacks
    # with nothing configured, the endpoint may 404 or return an empty doc.
    local body
    if body=$(curl -fsS "$GRAFANA_URL$path" 2>/dev/null); then
        echo "$body" > "$outfile"
        echo "   $(basename "$outfile")  ($label)"
    else
        # Leave the file as-is if it exists; otherwise write an empty stub so
        # the provisioning mount doesn't choke on a non-existent path.
        if [ ! -f "$outfile" ]; then
            printf 'apiVersion: 1\n' > "$outfile"
        fi
        echo "   $(basename "$outfile")  ($label — none configured)"
    fi
}

export_alerting "/api/v1/provisioning/alert-rules/export?format=yaml" \
    "$ALERT_DIR/rules.yaml" "alert rules"
export_alerting "/api/v1/provisioning/contact-points/export?format=yaml&decrypt=false" \
    "$ALERT_DIR/contact-points.yaml" "contact points"
export_alerting "/api/v1/provisioning/policies/export?format=yaml" \
    "$ALERT_DIR/policies.yaml" "notification policies"

echo ""
echo "Done. Review and commit with:"
echo "    git add $OUT_DIR"
echo "    git status $OUT_DIR"
