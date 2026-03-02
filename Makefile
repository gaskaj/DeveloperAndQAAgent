.PHONY: build test test-unit test-integration test-all test-cover test-race lint run clean fmt vet docker-test docker-test-performance docker-test-race

BINARY := agentctl
PKG := ./...
INTEGRATION_PKG := ./internal/integration/...

build:
	go build -o bin/$(BINARY) ./cmd/agentctl

# Unit tests (short, no integration tests)
test-unit:
	go test -race -count=1 -short $(PKG)

# Integration tests only
test-integration:
	@echo "Running integration tests..."
	@mkdir -p /tmp/test-workspaces /tmp/test-state
	INTEGRATION_TEST_MODE=true TEST_WORKSPACE_DIR=/tmp/test-workspaces TEST_STATE_DIR=/tmp/test-state \
		go test -v -race -timeout=30m -count=1 $(INTEGRATION_PKG)

# All tests (unit + integration)
test-all: test-unit test-integration

# Default test target (unit tests only for speed)
test: test-unit

# Race condition specific tests
test-race:
	@echo "Running race condition detection tests..."
	@mkdir -p /tmp/race-workspaces /tmp/race-state
	INTEGRATION_TEST_MODE=race TEST_WORKSPACE_DIR=/tmp/race-workspaces TEST_STATE_DIR=/tmp/race-state \
		go test -v -race -timeout=20m -count=3 $(INTEGRATION_PKG) -run="Race|Concurrent"

# Coverage for all tests
test-cover:
	@mkdir -p /tmp/test-workspaces /tmp/test-state
	INTEGRATION_TEST_MODE=true TEST_WORKSPACE_DIR=/tmp/test-workspaces TEST_STATE_DIR=/tmp/test-state \
		go test -race -coverprofile=coverage.out -timeout=35m $(PKG)
	go tool cover -html=coverage.out -o coverage.html

# Integration test coverage only
test-integration-cover:
	@mkdir -p /tmp/test-workspaces /tmp/test-state
	INTEGRATION_TEST_MODE=true TEST_WORKSPACE_DIR=/tmp/test-workspaces TEST_STATE_DIR=/tmp/test-state \
		go test -race -coverprofile=integration-coverage.out -timeout=30m $(INTEGRATION_PKG)
	go tool cover -html=integration-coverage.out -o integration-coverage.html

# Docker-based integration tests
docker-test:
	@echo "Running integration tests in Docker..."
	docker compose -f docker-compose.test.yml up --build --abort-on-container-exit test-runner
	docker compose -f docker-compose.test.yml down -v

# Docker-based performance tests  
docker-test-performance:
	@echo "Running performance integration tests in Docker..."
	docker compose -f docker-compose.test.yml up --build --abort-on-container-exit performance-tests
	docker compose -f docker-compose.test.yml down -v

# Docker-based race condition tests
docker-test-race:
	@echo "Running race condition tests in Docker..."
	docker compose -f docker-compose.test.yml up --build --abort-on-container-exit race-tests
	docker compose -f docker-compose.test.yml down -v

# Run all docker test suites
docker-test-all: docker-test docker-test-performance docker-test-race

lint: vet
	@which golangci-lint > /dev/null 2>&1 || echo "golangci-lint not installed"
	golangci-lint run $(PKG)

vet:
	go vet $(PKG)

fmt:
	gofmt -s -w .

run: build
	./bin/$(BINARY) start --config configs/config.example.yaml

clean:
	rm -rf bin/ coverage.out coverage.html integration-coverage.out integration-coverage.html
	rm -rf /tmp/test-workspaces /tmp/test-state /tmp/race-workspaces /tmp/race-state /tmp/perf-workspaces /tmp/perf-state
	docker compose -f docker-compose.test.yml down -v --remove-orphans || true

# Help target
help:
	@echo "Available targets:"
	@echo "  build                    - Build the binary"
	@echo "  test                     - Run unit tests (default)"
	@echo "  test-unit                - Run unit tests only"  
	@echo "  test-integration         - Run integration tests only"
	@echo "  test-all                 - Run all tests (unit + integration)"
	@echo "  test-race                - Run race condition detection tests"
	@echo "  test-cover               - Run all tests with coverage"
	@echo "  test-integration-cover   - Run integration tests with coverage"
	@echo "  docker-test              - Run integration tests in Docker"
	@echo "  docker-test-performance  - Run performance tests in Docker"
	@echo "  docker-test-race         - Run race condition tests in Docker"
	@echo "  docker-test-all          - Run all Docker test suites"
	@echo "  lint                     - Run linter"
	@echo "  fmt                      - Format code"
	@echo "  run                      - Build and run with example config"
	@echo "  clean                    - Clean build artifacts and test directories"
