#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
DURATION_MINUTES="${1:-2}"

NEST_PID=""
RECEIVER_PID=""
GENERATOR_PID=""

cleanup() {
    [[ -n "$GENERATOR_PID" ]] && kill "$GENERATOR_PID" 2>/dev/null || true
    [[ -n "$NEST_PID" ]] && kill "$NEST_PID" 2>/dev/null || true
    [[ -n "$RECEIVER_PID" ]] && kill "$RECEIVER_PID" 2>/dev/null || true
    rm -rf "$SCRIPT_DIR/bin"
}
trap cleanup EXIT

rm -f /tmp/nest_test_api_key.txt /tmp/nest_test_item_types.txt /tmp/nest_generator_stats.txt

for PORT in 8080 9090; do
    if lsof -ti :$PORT >/dev/null 2>&1; then
        echo "ERROR: Port $PORT is already in use."
        exit 1
    fi
done

echo "=== Nest Load Test (Timed) ==="
echo "Duration: ${DURATION_MINUTES} minute(s)"
echo ""

# Step 1: Start PostgreSQL
echo "[1/8] Starting PostgreSQL..."
docker compose -f "$SCRIPT_DIR/docker-compose.yml" up -d --wait

# Step 2: Build all binaries
echo "[2/8] Building binaries..."
mkdir -p "$SCRIPT_DIR/bin"
(cd "$PROJECT_DIR" && CGO_ENABLED=0 go build -o "$SCRIPT_DIR/bin/nest" ./cmd/server)
(cd "$PROJECT_DIR" && CGO_ENABLED=0 go build -o "$SCRIPT_DIR/bin/migrate" ./cmd/migrate)
(cd "$PROJECT_DIR" && CGO_ENABLED=0 go build -o "$SCRIPT_DIR/bin/seed" ./cmd/seed)
(cd "$SCRIPT_DIR/setup" && go build -o "$SCRIPT_DIR/bin/setup" .)
(cd "$SCRIPT_DIR/generator" && go build -o "$SCRIPT_DIR/bin/generator" .)
(cd "$SCRIPT_DIR/receiver" && go build -o "$SCRIPT_DIR/bin/receiver" .)
(cd "$SCRIPT_DIR/validate" && go build -o "$SCRIPT_DIR/bin/validate" .)

# Step 3: Migrate + seed + clean
echo "[3/8] Running migrations, seed, and cleaning test tables..."
export DATABASE_URL="postgres://nest:nest_test_pass@localhost:5433/nest_test?sslmode=disable"
"$SCRIPT_DIR/bin/migrate"
"$SCRIPT_DIR/bin/seed"

# Truncate test data tables so previous runs don't pollute counts.
# Cascade handles partitioned tables (rule_executions, action_executions).
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

# Step 6: Setup
echo "[6/8] Running test setup..."
"$SCRIPT_DIR/bin/setup" \
    -nest-url="http://localhost:8080" \
    -rules-dir="$SCRIPT_DIR/rules" \
    -webhook-url="http://localhost:9090/webhook"

# Step 7: Generator (unlimited, background) + timed sleep
echo "[7/8] Running generator for ${DURATION_MINUTES} minute(s)..."
API_KEY=$(cat /tmp/nest_test_api_key.txt)
"$SCRIPT_DIR/bin/generator" \
    -nest-url="http://localhost:8080" \
    -api-key="$API_KEY" \
    -max-items=0 \
    -rate=100 \
    -concurrency=10 &
GENERATOR_PID=$!

TOTAL_SECONDS=$(( DURATION_MINUTES * 60 ))
ELAPSED=0
while [[ "$ELAPSED" -lt "$TOTAL_SECONDS" ]]; do
    REMAINING=$(( TOTAL_SECONDS - ELAPSED ))
    echo "  [generator] Running... ${ELAPSED}s elapsed, ${REMAINING}s remaining"
    SLEEP_INTERVAL=30
    if [[ "$REMAINING" -lt "$SLEEP_INTERVAL" ]]; then
        SLEEP_INTERVAL="$REMAINING"
    fi
    sleep "$SLEEP_INTERVAL"
    ELAPSED=$(( ELAPSED + SLEEP_INTERVAL ))
done

echo "  Timer expired. Stopping generator..."
kill -TERM "$GENERATOR_PID" 2>/dev/null || true
wait "$GENERATOR_PID" 2>/dev/null || true
GENERATOR_PID=""

GEN_SENT=0; GEN_BLOCKS=0; GEN_REVIEWS=0
if [[ -f /tmp/nest_generator_stats.txt ]]; then
    GEN_SENT=$(grep '^sent=' /tmp/nest_generator_stats.txt | cut -d= -f2 || echo "0")
    GEN_BLOCKS=$(grep '^blocks=' /tmp/nest_generator_stats.txt | cut -d= -f2 || echo "0")
    GEN_REVIEWS=$(grep '^reviews=' /tmp/nest_generator_stats.txt | cut -d= -f2 || echo "0")
fi
echo "  Generator sent: $GEN_SENT items"

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
    -generator-blocks="$GEN_BLOCKS" \
    -generator-reviews="$GEN_REVIEWS" || VALIDATE_EXIT=$?

kill -TERM "$NEST_PID" 2>/dev/null || true
wait "$NEST_PID" 2>/dev/null || true
NEST_PID=""

kill -TERM "$RECEIVER_PID" 2>/dev/null || true
wait "$RECEIVER_PID" 2>/dev/null || true
RECEIVER_PID=""

exit $VALIDATE_EXIT
