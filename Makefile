.PHONY: db-up db-down build test lint run-api run-worker run-scheduler run-seeder run-dashboard chaos-demo proto

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
	go build -o bin/seeder ./cmd/seeder
	go build -o bin/obsidianctl ./cmd/obsidianctl

test:
	go test -v -count=1 ./...

lint:
	go vet ./...

# Protobuf Generation
proto:
	protoc --go_out=. --go_opt=module=obsidian --go-grpc_out=. --go-grpc_opt=module=obsidian proto/obsidian.proto

# Run Services
run-api:
	go run ./cmd/api

run-worker:
	go run ./cmd/worker

run-scheduler:
	go run ./cmd/scheduler

run-seeder:
	go run ./cmd/seeder

run-dashboard:
	cd dashboard && npm run dev

# Chaos Verification
chaos-demo:
	chmod +x ./chaos/kill-worker-demo.sh
	./chaos/kill-worker-demo.sh
