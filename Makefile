.PHONY: build build-controlplane test test-unit test-integration test-all test-cover test-race lint run clean fmt vet docker-test docker-test-performance docker-test-race coverage coverage-unit coverage-integration coverage-report coverage-badge coverage-analyze coverage-gates

BINARY := agentctl
PKG := ./...
INTEGRATION_PKG := ./internal/integration/...

build:
	go build -o bin/$(BINARY) ./cmd/agentctl

build-controlplane:
	go build -o bin/controlplane ./cmd/controlplane

build-all: build build-controlplane

# Unit tests (short, no integration tests)
test-unit:
	go test -race -count=1 -short $(PKG)

# Integration tests only
test-integration:
	@echo "Running integration tests..."
	@mkdir -p /tmp/test-workspaces /tmp/test-state
	INTEGRATION_TEST_MODE=true TEST_WORKSPACE_DIR=/tmp/test-workspaces TEST_STATE_DIR=/tmp/test-state \
		go test -v -race -timeout=30m -count=1 $(INTEGRATION_PKG)

# Contract tests
test-contract:
	@echo "Running API contract tests..."
	go test -v ./web/testing

# Frontend contract tests
test-contract-frontend:
	@echo "Running frontend contract tests..."
	cd frontend && npm run test:contract

# All contract validation
validate-contracts: test-contract test-contract-frontend
	@echo "Complete contract validation finished"

# All tests (unit + integration + contract)
test-all: test-unit test-integration test-contract

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

run-controlplane: build-controlplane
	./bin/controlplane --config configs/controlplane.example.yaml

run-frontend:
	cd frontend && npm run dev

clean:
	rm -rf bin/ coverage.out coverage.html integration-coverage.out integration-coverage.html
	rm -rf coverage-unit.out coverage-integration.out coverage-combined.out coverage-func.txt
	rm -rf coverage-report.md coverage-badge.svg coverage-badge-info.txt package-coverage-report.md
	rm -rf coverage-reports/ coverage-badge.md
	rm -rf /tmp/test-workspaces /tmp/test-state /tmp/race-workspaces /tmp/race-state /tmp/perf-workspaces /tmp/perf-state
	docker compose -f docker-compose.test.yml down -v --remove-orphans || true

# Comprehensive coverage targets
coverage: coverage-unit coverage-integration coverage-report coverage-badge
	@echo "Complete coverage analysis finished"

coverage-unit:
	@echo "Running unit tests with coverage..."
	@mkdir -p coverage-reports
	go test -short -race -coverprofile=coverage-unit.out -covermode=atomic ./internal/...

coverage-integration:
	@echo "Running integration tests with coverage..."
	@mkdir -p /tmp/test-workspaces /tmp/test-state coverage-reports
	INTEGRATION_TEST_MODE=true TEST_WORKSPACE_DIR=/tmp/test-workspaces TEST_STATE_DIR=/tmp/test-state \
		go test -race -coverprofile=coverage-integration.out -covermode=atomic -timeout=30m ./internal/integration/... || \
		(echo "mode: atomic" > coverage-integration.out && echo "No integration tests or tests failed")

coverage-combine:
	@echo "Combining coverage profiles..."
	@echo "mode: atomic" > coverage-combined.out
	@if [ -f coverage-unit.out ]; then tail -n +2 coverage-unit.out >> coverage-combined.out; fi
	@if [ -f coverage-integration.out ] && [ -s coverage-integration.out ]; then tail -n +2 coverage-integration.out >> coverage-combined.out; fi

coverage-html: coverage-combine
	@echo "Generating HTML coverage report..."
	go tool cover -html=coverage-combined.out -o coverage.html
	@echo "HTML coverage report: coverage.html"

coverage-func: coverage-combine
	@echo "Generating function coverage report..."
	go tool cover -func=coverage-combined.out > coverage-func.txt
	go tool cover -func=coverage-combined.out

coverage-report: coverage-html coverage-func
	@echo "Generating detailed coverage analysis..."
	@if [ -f scripts/coverage-report.go ]; then \
		go run scripts/coverage-report.go -profile=coverage-combined.out -format=markdown -output=coverage-report.md -verbose; \
	fi

coverage-badge: coverage-combine
	@echo "Generating coverage badge..."
	@if [ -x scripts/generate-coverage-badge.sh ]; then \
		./scripts/generate-coverage-badge.sh coverage-combined.out coverage-badge.svg README.md; \
	fi

coverage-analyze: coverage-combine
	@echo "Analyzing coverage against quality gates..."
	@if [ -f scripts/coverage-report.go ]; then \
		go run scripts/coverage-report.go -profile=coverage-combined.out -check-gates -verbose; \
	fi

coverage-gates: coverage-analyze
	@echo "Running quality gate validation..."
	@if [ -x scripts/coverage.sh ]; then \
		./scripts/coverage.sh analyze; \
	fi

# Enhanced test coverage with full analysis
test-coverage: coverage coverage-gates
	@echo "Complete test coverage analysis with quality gates"

# Help target
help:
	@echo "Available targets:"
	@echo ""
	@echo "Build and Run:"
	@echo "  build                    - Build the binary"
	@echo "  run                      - Build and run with example config"
	@echo ""
	@echo "Testing:"
	@echo "  test                     - Run unit tests (default)"
	@echo "  test-unit                - Run unit tests only"  
	@echo "  test-integration         - Run integration tests only"
	@echo "  test-contract            - Run API contract tests"
	@echo "  test-contract-frontend   - Run frontend contract tests"  
	@echo "  test-all                 - Run all tests (unit + integration + contract)"
	@echo "  test-race                - Run race condition detection tests"
	@echo "  test-cover               - Run all tests with coverage (legacy)"
	@echo "  test-integration-cover   - Run integration tests with coverage (legacy)"
	@echo "  validate-contracts       - Run complete contract validation (backend + frontend)"
	@echo ""
	@echo "Coverage Analysis:"
	@echo "  coverage                 - Complete coverage analysis (unit + integration + reports)"
	@echo "  coverage-unit            - Run unit tests with coverage"
	@echo "  coverage-integration     - Run integration tests with coverage"
	@echo "  coverage-combine         - Combine coverage profiles"
	@echo "  coverage-html            - Generate HTML coverage report"
	@echo "  coverage-func            - Generate function-level coverage report"
	@echo "  coverage-report          - Generate detailed coverage analysis"
	@echo "  coverage-badge           - Generate coverage badge for README"
	@echo "  coverage-analyze         - Analyze coverage against thresholds"
	@echo "  coverage-gates           - Run quality gate validation"
	@echo "  test-coverage            - Complete coverage analysis with quality gates"
	@echo ""
	@echo "Docker Testing:"
	@echo "  docker-test              - Run integration tests in Docker"
	@echo "  docker-test-performance  - Run performance tests in Docker"
	@echo "  docker-test-race         - Run race condition tests in Docker"
	@echo "  docker-test-all          - Run all Docker test suites"
	@echo ""
	@echo "Code Quality:"
	@echo "  lint                     - Run linter"
	@echo "  fmt                      - Format code"
	@echo "  vet                      - Run go vet"
	@echo ""
	@echo "Maintenance:"
	@echo "  clean                    - Clean build artifacts and test directories"
