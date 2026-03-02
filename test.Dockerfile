# Multi-stage Dockerfile for integration testing

# Base Go development image
FROM golang:1.25-alpine AS base
RUN apk add --no-cache git curl wget bash gcc musl-dev
ENV CGO_ENABLED=1
WORKDIR /workspace

# Test dependencies stage
FROM base AS test-deps
COPY go.mod go.sum ./
RUN go mod download
RUN go install github.com/onsi/ginkgo/v2/ginkgo@latest

# Test stage with all dependencies
FROM test-deps AS test
COPY . .

# Install additional testing tools
RUN go install golang.org/x/tools/cmd/goimports@latest
RUN go install honnef.co/go/tools/cmd/staticcheck@latest
RUN go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Create test directories
RUN mkdir -p /tmp/test-workspaces /tmp/test-state

# Default command
CMD ["go", "test", "-v", "./internal/integration/..."]