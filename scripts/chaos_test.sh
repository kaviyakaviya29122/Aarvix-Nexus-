#!/usr/bin/env bash
# Failure-recovery test: submit jobs, kill a worker mid-flight, verify every job still succeeds.
# Needs: docker compose stack running, curl, jq.   Usage: ADMIN_EMAIL=... ADMIN_PASSWORD=... ./scripts/chaos_test.sh
set -euo pipefail
API=${API:-http://localhost:8080}
EMAIL=${ADMIN_EMAIL:?set ADMIN_EMAIL}; PASS=${ADMIN_PASSWORD:?set ADMIN_PASSWORD}
N=${N:-12}

TOKEN=$(curl -sf -X POST "$API/api/auth/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\"}" | jq -r .token)
auth=(-H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json')

echo "== 1. submitting $N sleep jobs (8s each)"
ids=()
for i in $(seq 1 "$N"); do
  id=$(curl -sf -X POST "$API/api/jobs" "${auth[@]}" \
    -d "{\"type\":\"sleep\",\"payload\":{\"seconds\":8,\"n\":$i},\"max_attempts\":5}" | jq -r .job_id)
  ids+=("$id")
done

echo "== 2. killing worker1 while jobs are running"
sleep 3
docker compose kill worker1

echo "== 3. duplicate detection: same payload twice"
d1=$(curl -sf -X POST "$API/api/jobs" "${auth[@]}" -d '{"type":"echo","payload":{"x":1}}' | jq -r .duplicate)
d2=$(curl -sf -X POST "$API/api/jobs" "${auth[@]}" -d '{"type":"echo","payload":{"x":1}}' | jq -r .duplicate)
echo "first duplicate=$d1, second duplicate=$d2 (expected false/true)"

echo "== 4. waiting for recovery (heartbeat TTL 15s + lease handling)"
deadline=$((SECONDS + 120))
while :; do
  done_n=0
  for id in "${ids[@]}"; do
    s=$(curl -sf "$API/api/jobs/$id" "${auth[@]}" | jq -r .job.status)
    [ "$s" = "succeeded" ] && done_n=$((done_n + 1))
  done
  echo "   succeeded: $done_n/$N"
  [ "$done_n" -eq "$N" ] && break
  [ $SECONDS -gt $deadline ] && { echo "FAIL: not all jobs recovered"; docker compose start worker1; exit 1; }
  sleep 4
done

echo "== 5. recovery summary"
curl -sf "$API/api/admin/stats" "${auth[@]}" | jq '{recovered_attempts, duplicates_blocked, workers_online, workers_offline}'
docker compose start worker1
echo "PASS: all $N jobs completed despite worker failure"
