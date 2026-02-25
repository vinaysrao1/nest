#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
LOADTEST_DIR="$PROJECT_DIR/loadtest"
MAX_ITEMS="${1:-1000}"

NEST_PID=""
RECEIVER_PID=""

cleanup() {
    [[ -n "$NEST_PID" ]] && kill "$NEST_PID" 2>/dev/null || true
    [[ -n "$RECEIVER_PID" ]] && kill "$RECEIVER_PID" 2>/dev/null || true
    rm -rf "$SCRIPT_DIR/bin"
}
trap cleanup EXIT

rm -f /tmp/nest_test_api_key.txt /tmp/nest_test_item_types.txt /tmp/nest_consumer_stats.txt

for PORT in 8080 9090; do
    if lsof -ti :$PORT >/dev/null 2>&1; then
        echo "ERROR: Port $PORT is already in use."
        exit 1
    fi
done

echo "=== Nest Jetstream Integration Test ==="
echo "Max items: $MAX_ITEMS"
echo ""

# Step 1: Start PostgreSQL (shared docker-compose from loadtest)
echo "[1/8] Starting PostgreSQL..."
docker compose -f "$LOADTEST_DIR/docker-compose.yml" up -d --wait

# Step 2: Build binaries
echo "[2/8] Building binaries..."
mkdir -p "$SCRIPT_DIR/bin"
# Nest server binaries from main project
(cd "$PROJECT_DIR" && CGO_ENABLED=0 go build -o "$SCRIPT_DIR/bin/nest" ./cmd/server)
(cd "$PROJECT_DIR" && CGO_ENABLED=0 go build -o "$SCRIPT_DIR/bin/migrate" ./cmd/migrate)
(cd "$PROJECT_DIR" && CGO_ENABLED=0 go build -o "$SCRIPT_DIR/bin/seed" ./cmd/seed)
# Shared test infrastructure from loadtest/
(cd "$LOADTEST_DIR/setup" && go build -o "$SCRIPT_DIR/bin/setup" .)
(cd "$LOADTEST_DIR/receiver" && go build -o "$SCRIPT_DIR/bin/receiver" .)
(cd "$LOADTEST_DIR/validate" && go build -o "$SCRIPT_DIR/bin/validate" .)
# Jetstream consumer
(cd "$SCRIPT_DIR/consumer" && go build -o "$SCRIPT_DIR/bin/consumer" .)

# Step 3: Migrate + seed + clean
echo "[3/8] Running migrations, seed, and cleaning test tables..."
export DATABASE_URL="postgres://nest:nest_test_pass@localhost:5433/nest_test?sslmode=disable"
"$SCRIPT_DIR/bin/migrate"
"$SCRIPT_DIR/bin/seed"
docker exec loadtest-postgres-1 psql -U nest -d nest_test -c "TRUNCATE items, rule_executions, action_executions CASCADE;"
echo "  Cleaned test tables."

# Step 4: Start receiver
echo "[4/8] Starting webhook receiver on :9090..."
"$SCRIPT_DIR/bin/receiver" -addr=":9090" &
RECEIVER_PID=$!
sleep 1

# Step 5: Start Nest
echo "[5/8] Starting Nest on :8080..."
export PORT=8080
export SESSION_SECRET="test-session-secret-at-least-32-bytes-long"
export LOG_LEVEL=info
export WORKER_COUNT=4
# Optional: Set OPENAI_API_KEY to enable OpenAI moderation rules
# export OPENAI_API_KEY="${OPENAI_API_KEY:-}"
if [[ -n "${OPENAI_API_KEY:-}" ]]; then
    export OPENAI_API_KEY
fi
"$SCRIPT_DIR/bin/nest" &
NEST_PID=$!

READY=0
for i in $(seq 1 30); do
    if curl -sf http://localhost:8080/api/v1/health > /dev/null 2>&1; then
        READY=1
        break
    fi
    sleep 0.5
done
if [[ "$READY" -eq 0 ]]; then
    echo "ERROR: Nest did not become ready within 15 seconds"
    exit 1
fi
echo "  Nest ready."

# Step 6: Setup (uses shared rules from loadtest)
echo "[6/8] Running test setup..."
"$SCRIPT_DIR/bin/setup" \
    -nest-url="http://localhost:8080" \
    -rules-dir="$LOADTEST_DIR/rules" \
    -webhook-url="http://localhost:9090/webhook"

# Step 7: Consumer (real Jetstream firehose)
echo "[7/8] Running Jetstream consumer (max $MAX_ITEMS items)..."
API_KEY=$(cat /tmp/nest_test_api_key.txt)
"$SCRIPT_DIR/bin/consumer" \
    -nest-url="http://localhost:8080" \
    -api-key="$API_KEY" \
    -max-items="$MAX_ITEMS" \
    -batch-size=10

GEN_SENT=0
if [[ -f /tmp/nest_consumer_stats.txt ]]; then
    GEN_SENT=$(grep '^sent=' /tmp/nest_consumer_stats.txt | cut -d= -f2 || echo "0")
fi
echo "  Consumer sent: $GEN_SENT items"

# Step 8: Drain + validate
echo "[8/8] Waiting for pipeline drain (15 seconds)..."
sleep 15

echo ""
echo "=== Validation ==="
VALIDATE_EXIT=0
"$SCRIPT_DIR/bin/validate" \
    -receiver-url="http://localhost:9090" \
    -database-url="$DATABASE_URL" \
    -generator-sent="$GEN_SENT" \
    -generator-blocks=0 \
    -generator-reviews=0 || VALIDATE_EXIT=$?

kill -TERM "$NEST_PID" 2>/dev/null || true
wait "$NEST_PID" 2>/dev/null || true
NEST_PID=""

kill -TERM "$RECEIVER_PID" 2>/dev/null || true
wait "$RECEIVER_PID" 2>/dev/null || true
RECEIVER_PID=""

exit $VALIDATE_EXIT
