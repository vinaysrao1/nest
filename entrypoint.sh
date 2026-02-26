#!/bin/sh
set -e

echo "[entrypoint] Starting Nest demo..."

# ---------------------------------------------------------------------------
# 1. Initialize PostgreSQL
# ---------------------------------------------------------------------------
PGDATA="/var/lib/postgresql/data"

if [ ! -f "$PGDATA/PG_VERSION" ]; then
    echo "[entrypoint] Initializing PostgreSQL data directory..."
    su-exec postgres initdb -D "$PGDATA" --auth=trust --no-locale --encoding=UTF8 > /dev/null

    # Allow local TCP connections without password for the demo
    echo "host all all 127.0.0.1/32 trust" >> "$PGDATA/pg_hba.conf"
    echo "host all all ::1/128 trust" >> "$PGDATA/pg_hba.conf"
fi

# ---------------------------------------------------------------------------
# 2. Start PostgreSQL
# ---------------------------------------------------------------------------
echo "[entrypoint] Starting PostgreSQL..."
su-exec postgres pg_ctl -D "$PGDATA" -l /var/lib/postgresql/pg.log start -o "-k /tmp" > /dev/null

echo "[entrypoint] Waiting for PostgreSQL..."
until su-exec postgres pg_isready -h localhost -q; do
    sleep 0.5
done
echo "[entrypoint] PostgreSQL is ready."

# ---------------------------------------------------------------------------
# 3. Create database and role
# ---------------------------------------------------------------------------
su-exec postgres psql -h localhost -c "SELECT 1 FROM pg_roles WHERE rolname='nest'" | grep -q 1 || \
    su-exec postgres psql -h localhost -c "CREATE ROLE nest WITH LOGIN PASSWORD 'nest';"

su-exec postgres psql -h localhost -tc "SELECT 1 FROM pg_database WHERE datname='nest'" | grep -q 1 || \
    su-exec postgres psql -h localhost -c "CREATE DATABASE nest OWNER nest;"

echo "[entrypoint] Database and role ready."

# ---------------------------------------------------------------------------
# 4. Run migrations
# ---------------------------------------------------------------------------
echo "[entrypoint] Running migrations..."
/app/migrate
echo "[entrypoint] Migrations complete."

# ---------------------------------------------------------------------------
# 5. Seed data (org, admin user, MRT queues)
# ---------------------------------------------------------------------------
echo "[entrypoint] Seeding data..."
/app/seed
echo "[entrypoint] Seed complete."

# ---------------------------------------------------------------------------
# 6. Start Nest server in background
# ---------------------------------------------------------------------------
echo "[entrypoint] Starting Nest server on port ${PORT:-8080}..."
/app/nest &
NEST_PID=$!

echo "[entrypoint] Waiting for Nest server..."
until curl -sf http://localhost:${PORT:-8080}/api/v1/health > /dev/null 2>&1; do
    sleep 0.5
done
echo "[entrypoint] Nest server is ready."

# ---------------------------------------------------------------------------
# 7. Run setup (create item types, rules, actions, API key)
# ---------------------------------------------------------------------------
echo "[entrypoint] Running setup..."
/app/setup -nest-url "http://localhost:${PORT:-8080}" -rules-dir /app/rules -webhook-url "http://localhost:9090/webhook"
echo "[entrypoint] Setup complete."

# ---------------------------------------------------------------------------
# 8. Start Jetstream consumer in background
# ---------------------------------------------------------------------------
echo "[entrypoint] Starting Jetstream consumer..."
/app/consumer &
CONSUMER_PID=$!

# ---------------------------------------------------------------------------
# 9. Start Python UI in background
# ---------------------------------------------------------------------------
echo "[entrypoint] Starting NiceGUI admin UI on port ${UI_PORT:-8090}..."
cd /app/nest-ui
su-exec nest python3 main.py &
UI_PID=$!
cd /app

echo "[entrypoint] All services started."
echo "[entrypoint]   Nest API:  http://localhost:${PORT:-8080}"
echo "[entrypoint]   Admin UI:  http://localhost:${UI_PORT:-8090}"
echo "[entrypoint]   Login:     admin@nest.local / admin123"

# ---------------------------------------------------------------------------
# 10. Wait for any process to exit — if one dies, stop everything
# ---------------------------------------------------------------------------
wait -n $NEST_PID $CONSUMER_PID $UI_PID 2>/dev/null || true

echo "[entrypoint] A process exited. Shutting down..."
kill $NEST_PID $CONSUMER_PID $UI_PID 2>/dev/null || true
wait 2>/dev/null || true
