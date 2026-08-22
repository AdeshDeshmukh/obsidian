.PHONY: db-up db-down build test run-api run-worker run-scheduler run-dashboard chaos-demo

# Database
db-up:
	docker compose up -d

db-down:
	docker compose down

# Compilation & Testing
build:
	go build -o bin/api ./cmd/api
	go build -o bin/worker ./cmd/worker
	go build -o bin/scheduler ./cmd/scheduler

test:
	go test -v ./internal/...

# Run Services
run-api:
	go run ./cmd/api

run-worker:
	go run ./cmd/worker

run-scheduler:
	go run ./cmd/scheduler

run-dashboard:
	cd dashboard && npm run dev

# Chaos Verification
chaos-demo:
	chmod +x ./chaos/kill-worker-demo.sh
	./chaos/kill-worker-demo.sh
