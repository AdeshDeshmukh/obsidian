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

# 5. Enqueue a 20s long-running sleep job
echo "5. Dispatching a 20-second sleep job..."
JOB_RES=$(curl -s -X POST "${API_URL}/api/queues/${QUEUE_ID}/jobs" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"job_type":"sleep","payload":{"seconds":20},"priority":10}')
JOB_ID=$(echo "$JOB_RES" | grep -o '"id":"[^"]*' | grep -o '[^"]*$')
echo "Job created with ID: ${JOB_ID}"

sleep 3
STATUS_RES=$(curl -s "${API_URL}/api/jobs/${JOB_ID}" -H "Authorization: Bearer ${TOKEN}")
STATUS=$(echo "$STATUS_RES" | grep -o '"status":"[^"]*' | grep -o '[^"]*$')
echo "Current Job Status: ${STATUS} (Expected: running)"

# 6. Kill worker mid-execution!
echo "6. CRASHING WORKER NODE MID-EXECUTION (sending SIGKILL)..."
kill -9 "${WORKER_PID}"
echo "Worker crashed."

echo "7. Monitoring lease recovery. The scheduler lease timeout is set to 30s."
echo "Waiting for lease to expire and scheduler to return the job to 'queued' state..."

for i in {1..40}; do
  STATUS_RES=$(curl -s "${API_URL}/api/jobs/${JOB_ID}" -H "Authorization: Bearer ${TOKEN}")
  STATUS=$(echo "$STATUS_RES" | grep -o '"status":"[^"]*' | grep -o '[^"]*$')
  echo "[Tick $i] Status is: ${STATUS}"
  
  if [ "$STATUS" = "queued" ]; then
    echo "=========================================================="
    echo " SUCCESS: Job reclaimed automatically by lease recovery!"
    echo "=========================================================="
    exit 0
  fi
  sleep 2
done

echo "Fail: Job was not reclaimed within timeout."
exit 1
