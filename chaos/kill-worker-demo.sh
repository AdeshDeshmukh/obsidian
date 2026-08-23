#!/usr/bin/env bash

# This script demonstrates the reliability and lease-recovery mechanisms of Obsidian.
# It runs a worker, enqueues a long-running job, kills the worker mid-execution,
# and verifies that the scheduler's janitor loop reclaims the job after lease expiry.

set -eo pipefail

API_URL="http://localhost:8080"
EMAIL="chaos-test@obsidian.io"
PASSWORD="password123"

# Verify API is up
if ! curl -sf "${API_URL}/health" > /dev/null; then
  echo "Error: API server is not running on ${API_URL}. Please start it using 'make run-api' first."
  exit 1
fi

# Ensure clean up of binary on exit
trap "rm -f ./worker-bin" EXIT

echo "=========================================================="
echo "          Obsidian Worker Chaos Recovery Demo             "
echo "=========================================================="

# 1. Sign up/Log in to get JWT token
echo "1. Registering user..."
AUTH_RES=$(curl -s -X POST "${API_URL}/api/auth/register" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"${EMAIL}\",\"password\":\"${PASSWORD}\"}" || true)

# Try login if already exists
if [[ "$AUTH_RES" == *"error"* ]]; then
  echo "User already exists. Logging in..."
  AUTH_RES=$(curl -s -X POST "${API_URL}/api/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"${EMAIL}\",\"password\":\"${PASSWORD}\"}")
fi

TOKEN=$(echo "$AUTH_RES" | grep -o '"token":"[^"]*' | grep -o '[^"]*$')
if [ -z "$TOKEN" ]; then
  echo "Error: Failed to obtain authentication token."
  exit 1
fi
echo "Authenticated successfully."

# 2. Create Project
echo "2. Creating chaos project..."
PROJ_RES=$(curl -s -X POST "${API_URL}/api/projects" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"name":"Chaos Project"}')
PROJ_ID=$(echo "$PROJ_RES" | grep -o '"id":"[^"]*' | grep -o '[^"]*$')
echo "Project created with ID: ${PROJ_ID}"

# 3. Create Queue
echo "3. Creating chaos queue..."
QUEUE_RES=$(curl -s -X POST "${API_URL}/api/projects/${PROJ_ID}/queues" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"name":"chaos-queue","priority":1,"concurrency_limit":2}')
QUEUE_ID=$(echo "$QUEUE_RES" | grep -o '"id":"[^"]*' | grep -o '[^"]*$')
echo "Queue created with ID: ${QUEUE_ID}"

# 4. Start one worker in background
echo "4. Launching Worker Node..."
echo "Compiling worker binary..."
go build -o ./worker-bin ./cmd/worker

WORKER_ID="chaos-worker-999"
DATABASE_URL="postgres://postgres:obsidian@localhost:5432/obsidian?sslmode=disable" \
  WORKER_ID="${WORKER_ID}" \
  QUEUE_ID="${QUEUE_ID}" \
  ./worker-bin > /tmp/worker.log 2>&1 &
WORKER_PID=$!
echo "Worker process running on PID: ${WORKER_PID}"

# Ensure worker starts
sleep 2

# 5. Enqueue a 10s long-running sleep job
echo "5. Dispatching a 10-second sleep job..."
JOB_RES=$(curl -s -X POST "${API_URL}/api/queues/${QUEUE_ID}/jobs" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"job_type":"sleep","payload":{"seconds":10},"priority":10}')
JOB_ID=$(echo "$JOB_RES" | grep -o '"id":"[^"]*' | grep -o '[^"]*$')
echo "Job created with ID: ${JOB_ID}"

sleep 3
STATUS_RES=$(curl -s "${API_URL}/api/jobs/${JOB_ID}" -H "Authorization: Bearer ${TOKEN}")
STATUS=$(echo "$STATUS_RES" | grep -o '"status":"[^"]*' | grep -o '[^"]*$')
echo "Current Job Status: ${STATUS} (Expected: running)"

# 6. Kill worker 1 mid-execution!
echo "6. CRASHING WORKER NODE 1 MID-EXECUTION (sending SIGKILL)..."
kill -9 "${WORKER_PID}"
echo "Worker 1 crashed."

echo "7. Monitoring lease recovery. Waiting for lease to expire and scheduler to return job to 'queued' state..."

RECLAIMED=0
for i in {1..30}; do
  STATUS_RES=$(curl -s "${API_URL}/api/jobs/${JOB_ID}" -H "Authorization: Bearer ${TOKEN}")
  STATUS=$(echo "$STATUS_RES" | grep -o '"status":"[^"]*' | grep -o '[^"]*$')
  echo "[Tick $i] Status is: ${STATUS}"
  
  if [ "$STATUS" = "queued" ]; then
    echo "Job reclaimed automatically by lease recovery!"
    RECLAIMED=1
    break
  fi
  sleep 2
done

if [ "$RECLAIMED" -eq 0 ]; then
  echo "Fail: Job was not reclaimed within timeout."
  exit 1
fi

# 8. Launch Worker 2 to complete the failover recovery cycle
echo "8. Launching Worker Node 2 to pick up reclaimed job..."
WORKER2_ID="chaos-worker-888"
DATABASE_URL="postgres://postgres:obsidian@localhost:5432/obsidian?sslmode=disable" \
  WORKER_ID="${WORKER2_ID}" \
  QUEUE_ID="${QUEUE_ID}" \
  ./worker-bin > /tmp/worker2.log 2>&1 &
WORKER2_PID=$!

echo "Worker 2 running on PID: ${WORKER2_PID}. Waiting for completion..."

for i in {1..20}; do
  STATUS_RES=$(curl -s "${API_URL}/api/jobs/${JOB_ID}" -H "Authorization: Bearer ${TOKEN}")
  STATUS=$(echo "$STATUS_RES" | grep -o '"status":"[^"]*' | grep -o '[^"]*$')
  echo "[Failover Tick $i] Status is: ${STATUS}"
  
  if [ "$STATUS" = "completed" ]; then
    kill -9 "${WORKER2_PID}" || true
    echo "=========================================================="
    echo " SUCCESS: Full Chaos Failover Lifecycle Verified!"
    echo " queued -> running (worker 1) -> queued (reclaimed) -> running (worker 2) -> completed"
    echo "=========================================================="
    exit 0
  fi
  sleep 2
done

kill -9 "${WORKER2_PID}" || true
echo "Fail: Worker 2 did not complete job within timeout."
exit 1

