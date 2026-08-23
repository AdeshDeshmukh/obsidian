# Obsidian API Specifications

All endpoints are hosted relative to the base URL: `http://localhost:8080`.
Private endpoints require the header: `Authorization: Bearer <JWT_TOKEN>`.

---

## 1. Authentication

### Register Account
*   **Endpoint**: `POST /api/auth/register`
*   **Headers**: `Content-Type: application/json`
*   **Body**:
    ```json
    {
      "email": "user@example.com",
      "password": "securepassword123"
    }
    ```
*   **Response (201 Created)**:
    ```json
    {
      "token": "eyJhbGciOiJIUzI1NiIsIn...",
      "userId": "d70e4c63-4613-43bb-a5a5-48beab4bf6f3"
    }
    ```

### Login Account
*   **Endpoint**: `POST /api/auth/login`
*   **Headers**: `Content-Type: application/json`
*   **Body**:
    ```json
    {
      "email": "user@example.com",
      "password": "securepassword123"
    }
    ```
*   **Response (200 OK)**:
    ```json
    {
      "token": "eyJhbGciOiJIUzI1NiIsIn...",
      "userId": "d70e4c63-4613-43bb-a5a5-48beab4bf6f3"
    }
    ```

---

## 2. Projects & Queues

### Create Project
*   **Endpoint**: `POST /api/projects`
*   **Headers**: `Authorization: Bearer <token>`
*   **Body**:
    ```json
    {
      "name": "Production Jobs"
    }
    ```
*   **Response (201 Created)**:
    ```json
    {
      "id": "e5bfa78a-db55-4dbb-9721-a4b5ff50b86a",
      "name": "Production Jobs",
      "apiKey": "apiKey_1724283859000_d70e4c63"
    }
    ```

### List Projects
*   **Endpoint**: `GET /api/projects`
*   **Headers**: `Authorization: Bearer <token>`
*   **Response (200 OK)**:
    ```json
    [
      {
        "id": "e5bfa78a-db55-4dbb-9721-a4b5ff50b86a",
        "name": "Production Jobs",
        "api_key": "apiKey_1724283859000_d70e4c63",
        "created_at": "2026-08-22T00:30:00Z"
      }
    ]
    ```

### Create Queue
*   **Endpoint**: `POST /api/projects/{projectId}/queues`
*   **Headers**: `Authorization: Bearer <token>`
*   **Body**:
    ```json
    {
      "name": "email-notifications",
      "priority": 10,
      "concurrency_limit": 5
    }
    ```
*   **Response (201 Created)**:
    ```json
    {
      "id": "f5cb78b9-8e7c-48ab-9cba-115f01beaa2b",
      "name": "email-notifications",
      "priority": 10,
      "concurrency_limit": 5
    }
    ```

### List Queues
*   **Endpoint**: `GET /api/projects/{projectId}/queues`
*   **Headers**: `Authorization: Bearer <token>`
*   **Response (200 OK)**:
    ```json
    [
      {
        "id": "f5cb78b9-8e7c-48ab-9cba-115f01beaa2b",
        "name": "email-notifications",
        "priority": 10,
        "concurrency_limit": 5,
        "is_paused": false,
        "created_at": "2026-08-22T00:31:00Z"
      }
    ]
    ```

### Update Queue Status / Priority / Concurrency
*   **Endpoint**: `PUT /api/queues/{queueId}`
*   **Headers**: `Authorization: Bearer <token>`
*   **Body**:
    ```json
    {
      "priority": 20,
      "concurrency_limit": 8,
      "is_paused": true
    }
    ```
*   **Response (200 OK)**:
    ```json
    {
      "id": "f5cb78b9-8e7c-48ab-9cba-115f01beaa2b",
      "priority": 20,
      "concurrency_limit": 8,
      "is_paused": true
    }
    ```

---

## 3. Job Dispatching & Management

### Create Immediate / Delayed / Cron Job
*   **Endpoint**: `POST /api/queues/{queueId}/jobs`
*   **Headers**: `Authorization: Bearer <token>`
*   **Body (Immediate)**:
    ```json
    {
      "job_type": "log",
      "payload": {
        "message": "Immediate logging test"
      },
      "priority": 5
    }
    ```
*   **Body (Delayed 10 minutes)**:
    ```json
    {
      "job_type": "noop",
      "payload": {},
      "run_at": "2026-08-22T00:40:00Z"
    }
    ```
*   **Body (Recurring Cron - every 5 minutes)**:
    ```json
    {
      "job_type": "noop",
      "payload": {},
      "cron_expr": "*/5 * * * *"
    }
    ```
*   **Body (Job Dependencies - DAG)**:
    ```json
    {
      "job_type": "noop",
      "payload": {},
      "depends_on": ["e1c4b8b9-8e7c-48ab-9cba-115f01beaa2b"]
    }
    ```
*   **Response (201 Created)**:
    ```json
    {
      "id": "a9cb78c9-8e7c-48ab-9cba-115f01beaa2b",
      "status": "queued",
      "run_at": "2026-08-22T00:30:00Z"
    }
    ```

### List Jobs
*   **Endpoint**: `GET /api/queues/{queueId}/jobs`
*   **Params**: `?status=failed&job_type=log` (optional)
*   **Headers**: `Authorization: Bearer <token>`
*   **Response (200 OK)**:
    ```json
    [
      {
        "id": "a9cb78c9-8e7c-48ab-9cba-115f01beaa2b",
        "job_type": "log",
        "status": "failed",
        "priority": 5,
        "run_at": "2026-08-22T00:30:00Z",
        "attempt": 3,
        "created_at": "2026-08-22T00:30:00Z",
        "updated_at": "2026-08-22T00:32:00Z"
      }
    ]
    ```

### Inspect Single Job (Logs + History)
*   **Endpoint**: `GET /api/jobs/{jobId}`
*   **Headers**: `Authorization: Bearer <token>`
*   **Response (200 OK)**:
    ```json
    {
      "id": "a9cb78c9-8e7c-48ab-9cba-115f01beaa2b",
      "queue_id": "f5cb78b9-8e7c-48ab-9cba-115f01beaa2b",
      "job_type": "log",
      "payload": {
        "message": "Immediate logging test"
      },
      "status": "completed",
      "priority": 5,
      "run_at": "2026-08-22T00:30:00Z",
      "attempt": 1,
      "claimed_by": "worker-150405-9271",
      "lease_expires_at": "2026-08-22T00:30:30Z",
      "created_at": "2026-08-22T00:30:00Z",
      "updated_at": "2026-08-22T00:30:04Z",
      "executions": [
        {
          "id": "b0cb78d9-8e7c-48ab-9cba-115f01beaa2b",
          "worker_id": "worker-150405-9271",
          "attempt": 0,
          "started_at": "2026-08-22T00:30:01Z",
          "finished_at": "2026-08-22T00:30:04Z",
          "outcome": "success",
          "error_message": null,
          "duration_ms": 3000
        }
      ],
      "logs": [
        {
          "level": "info",
          "message": "Job created and enqueued",
          "logged_at": "2026-08-22T00:30:00Z"
        },
        {
          "level": "info",
          "message": "Job completed successfully",
          "logged_at": "2026-08-22T00:30:04Z"
        }
      ]
    }
    ```

### Retry Failed / DLQ Job Manually
*   **Endpoint**: `POST /api/jobs/{jobId}/retry`
*   **Headers**: `Authorization: Bearer <token>`
*   **Response (200 OK)**:
    ```json
    {
      "message": "job enqueued for retry"
    }
    ```

### Cancel Queued / Scheduled Job
*   **Endpoint**: `POST /api/jobs/{jobId}/cancel`
*   **Headers**: `Authorization: Bearer <token>`
*   **Response (200 OK)**:
    ```json
    {
      "message": "job cancelled successfully"
    }
    ```

---

## 4. Observability & Workers

### List Workers
*   **Endpoint**: `GET /api/workers`
*   **Headers**: `Authorization: Bearer <token>`
*   **Response (200 OK)**:
    ```json
    [
      {
        "id": "worker-150405-9271",
        "hostname": "local-developer-macbook.local",
        "started_at": "2026-08-22T00:25:00Z",
        "last_seen_at": "2026-08-22T00:34:00Z",
        "status": "active"
      }
    ]
    ```

### Get System Health Metrics
*   **Endpoint**: `GET /api/metrics/system-health`
*   **Headers**: `Authorization: Bearer <token>`
*   **Response (200 OK)**:
    ```json
    {
      "queue_sizes": [
        {
          "queue_id": "f5cb78b9-8e7c-48ab-9cba-115f01beaa2b",
          "name": "email-notifications",
          "size": 4
        }
      ],
      "active_workers": 1,
      "failed_count": 2,
      "success_count": 48,
      "dlq_count": 1
    }
    ```
