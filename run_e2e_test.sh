#!/usr/bin/env bash
# End-to-End Test Script for QuakeAlert
# This script performs the complete test flow:
# 1. Start server
# 2. Seed history data
# 3. Provision IoT nodes
# 4. Run MQTT simulation
# 5. Verify results

set -euo pipefail

PROJECT_ROOT="/home/vitowiratara/QuakeAlert26"
SERVER_DIR="$PROJECT_ROOT/server"
SCRIPTS_DIR="$SERVER_DIR/scripts"

# Step 0: Reset database
echo "=== Step 0: Resetting database ==="
docker exec -i quakealert-postgis psql -U quakealert -d quakealert <<'EOSQL'
TRUNCATE iot_nodes CASCADE;
TRUNCATE earthquake_events CASCADE;
SELECT 'Database reset complete';
EOSQL

# Step 1: Generate consistent keys
echo ""
echo "=== Step 1: Generating keys ==="
MASTER_KEY_HEX=$(openssl rand -hex 32)
JWT_SECRET=$(openssl rand -hex 32)
export MASTER_KEY_HEX
export JWT_SECRET
echo "MASTER_KEY_HEX=$MASTER_KEY_HEX"
echo "JWT_SECRET length: ${#JWT_SECRET}"

# Step 2: Start server in background
echo ""
echo "=== Step 2: Starting server ==="
cd "$SERVER_DIR"
nohup /tmp/quakealert </dev/null >"/tmp/server_e2e.log" 2>&1 &
SERVER_PID=$!
echo "Server PID: $SERVER_PID"

# Wait for server ready
for i in {1..40}; do
  if curl -fsS http://localhost:8080/healthz >/dev/null 2>&1; then
    echo "Server ready after $i seconds"
    break
  fi
  sleep 1
done

# Step 3: Seed history data
echo ""
echo "=== Step 3: Seeding history data ==="
docker exec -i quakealert-postgis psql -U quakealert -d quakealert <<'EOSQL'
INSERT INTO earthquake_events (status, estimated_centroid, location_name, mmi_scale, intensity_label, max_pga, triggered_nodes_count, started_at, resolved_at) VALUES
  ('RESOLVED', ST_SetSRID(ST_MakePoint(107.61, -6.91), 4326)::geography, 'Bandung, West Java', 'V', 'Moderate', 85.4200, 4, NOW() - INTERVAL '2 days', NOW() - INTERVAL '2 days' + INTERVAL '90 seconds'),
  ('RESOLVED', ST_SetSRID(ST_MakePoint(107.58, -6.93), 4326)::geography, 'Cimahi, West Java', 'IV', 'Light', 42.1500, 3, NOW() - INTERVAL '5 days', NOW() - INTERVAL '5 days' + INTERVAL '90 seconds'),
  ('RESOLVED', ST_SetSRID(ST_MakePoint(107.65, -6.88), 4326)::geography, 'Lembang, West Java', 'VI', 'Strong', 185.3000, 5, NOW() - INTERVAL '10 days', NOW() - INTERVAL '10 days' + INTERVAL '90 seconds'),
  ('HAPPENING', ST_SetSRID(ST_MakePoint(107.60, -6.90), 4326)::geography, 'Bandung Selatan', 'IV', 'Light', 38.7500, 3, NOW() - INTERVAL '30 minutes', NULL);
EOSQL
echo "History data seeded"

# Step 4: Get auth token and provision nodes (do this immediately, before rate limiter kicks in)
echo ""
echo "=== Step 4: Provisioning IoT nodes ==="
AUTH_RESULT=$(curl -s -X POST http://localhost:8080/api/v1/auth/anonymous)
TOKEN=$(echo "$AUTH_RESULT" | grep -o '"token": *"[^"]*"' | cut -d'"' -f4)
echo "Got token: ${#TOKEN} chars"

# Provision Node 1
N1=$(curl -s -X POST "http://localhost:8080/api/v1/nodes/provision" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"station_id":"NODE-AAAAAAAA","sensor_model":"MPU 6050","location_name":"Bandung Station 1","latitude":-6.91,"longitude":107.61}')
S1=$(echo "$N1" | grep -o '"provisioning_secret": *"[^"]*"' | cut -d'"' -f4)
echo "Node 1: ${S1:0:30}..."

# Provision Node 2
N2=$(curl -s -X POST "http://localhost:8080/api/v1/nodes/provision" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"station_id":"NODE-BBBBBBBB","sensor_model":"MPU 6050","location_name":"Bandung Station 2","latitude":-6.92,"longitude":107.62}')
S2=$(echo "$N2" | grep -o '"provisioning_secret": *"[^"]*"' | cut -d'"' -f4)
echo "Node 2: ${S2:0:30}..."

# Provision Node 3
N3=$(curl -s -X POST "http://localhost:8080/api/v1/nodes/provision" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"station_id":"NODE-CCCCCCCC","sensor_model":"MPU 6050","location_name":"Bandung Station 3","latitude":-6.90,"longitude":107.60}')
S3=$(echo "$N3" | grep -o '"provisioning_secret": *"[^"]*"' | cut -d'"' -f4)
echo "Node 3: ${S3:0:30}..."

# Save secrets to file
echo "$S1 $S2 $S3" > /tmp/e2e_secrets.txt
echo "Secrets saved to /tmp/e2e_secrets.txt"

# Step 5: Run MQTT simulation
echo ""
echo "=== Step 5: Running MQTT simulation ==="
cd "$SERVER_DIR"
go run "$SCRIPTS_DIR/simulate_trigger.go" $(cat /tmp/e2e_secrets.txt)
SIM_RESULT=$?

# Step 6: Verify results
echo ""
echo "=== Step 6: Verifying results ==="
echo "History events:"
docker exec -i quakealert-postgis psql -U quakealert -d quakealert -c "SELECT event_id, status, location_name, mmi_scale, max_pga, started_at FROM earthquake_events ORDER BY started_at DESC;" 2>&1 | head -10

echo ""
echo "Server logs for 'event didispatch':"
grep -c "event didispatch" /tmp/server_e2e.log 2>/dev/null || echo "0 occurrences"
grep "event didispatch" /tmp/server_e2e.log 2>/dev/null | tail -5 || echo "No dispatches found"

echo ""
if [ $SIM_RESULT -eq 0 ]; then
  echo "=== E2E TEST COMPLETE ==="
  echo "Simulation ran successfully. Check the Android tabs:"
  echo "  - History tab: should show 4 seeded events"
  echo "  - Warning tab: should show EARTHQUAKE_ALERT broadcast via WebSocket"
else
  echo "=== SIMULATION FAILED ==="
  exit 1
fi

# Cleanup: stop server
echo ""
echo "=== Cleanup: Stopping server ==="
kill $SERVER_PID 2>/dev/null || true
sleep 2
docker compose -f "$SERVER_DIR/docker-compose.yml" down >/dev/null 2>&1 || true
echo "Test complete"