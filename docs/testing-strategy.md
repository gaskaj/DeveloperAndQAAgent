# Testing Strategy

This document outlines the comprehensive testing strategy for the OpenAgentFramework project, including unit tests, integration tests, coverage requirements, and quality assurance practices.

## Testing Philosophy

Our testing approach is built on these core principles:

1. **Reliability First**: Critical code paths must have comprehensive test coverage
2. **Fast Feedback**: Unit tests run quickly to enable rapid development cycles
3. **Real-World Validation**: Integration tests verify system behavior in realistic scenarios
4. **Quality Gates**: Automated enforcement of testing standards prevents regressions
5. **Continuous Improvement**: Coverage and quality metrics guide testing investments

## Testing Pyramid

```
        ┌─────────────────┐
        │  Manual Tests   │
        │   (Minimal)     │
        ├─────────────────┤
        │ Integration     │ 
        │    Tests        │
        │  (Moderate)     │
        ├─────────────────┤
        │   Unit Tests    │
        │ (Comprehensive) │
        └─────────────────┘
```

### Test Distribution

- **Unit Tests (70-80%)**: Fast, isolated tests for individual functions/methods
- **Integration Tests (20-30%)**: End-to-end scenarios and API interactions
- **Manual Tests (Minimal)**: Exploratory testing and edge case validation

## Testing Framework and Tools

### Core Testing Stack

| Tool | Purpose | Usage |
|------|---------|-------|
| `testing` | Go standard testing framework | All unit tests |
| `testify` | Assertions and test utilities | Enhanced test readability |
| `testify/mock` | Mock generation and verification | Dependency isolation |
| `go test -race` | Race condition detection | Concurrent code validation |
| `go test -cover` | Coverage analysis | Quality metrics |

### Test Environment

- **Local Development**: Fast unit test execution
- **CI/CD Pipeline**: Full test suite with coverage analysis
- **Docker Environment**: Isolated integration testing
- **Mock Services**: External dependency simulation

## Testing Categories

### Unit Tests

**Purpose**: Test individual functions, methods, and components in isolation.

**Characteristics**:
- Fast execution (< 1 second per test)
- No external dependencies
- Deterministic and repeatable
- High code coverage focus

**Scope**:
```go
// Example: Testing business logic
func TestWorkflowStateTransition(t *testing.T) {
    workflow := NewWorkflow()
    
    err := workflow.Transition(StateIdle, EventStart)
    
    assert.NoError(t, err)
    assert.Equal(t, StateRunning, workflow.CurrentState())
}
```

**Best Practices**:
- Use table-driven tests for multiple scenarios
- Mock external dependencies
- Test both success and error paths
- Focus on business logic and edge cases

### Integration Tests

**Purpose**: Test interactions between components and external systems.

**Location**: `./internal/integration/`

**Characteristics**:
- Slower execution (seconds to minutes)
- Uses real or near-real dependencies
- Tests end-to-end workflows
- Validates system integration

**Test Categories**:

1. **Agent Communication** (`agent_communication_test.go`)
   - Message protocol compliance
   - Concurrent agent processing
   - Timeout and retry handling
   - State consistency across agents

2. **Workflow Handoffs** (`workflow_handoff_test.go`)
   - Developer-to-QA transitions
   - Context preservation
   - Error handling during handoffs
   - Performance of handoff operations

3. **Shared State Management** (`shared_state_test.go`)
   - Concurrent state access
   - Resource contention handling
   - Race condition prevention
   - State consistency under failure

4. **System Integration** (`simple_agent_test.go`)
   - Full agent lifecycle
   - External API integration
   - Mock service contracts
   - Error simulation and recovery

### Race Condition Tests

**Purpose**: Detect and prevent race conditions in concurrent code.

**Execution**: `go test -race`

**Focus Areas**:
- Shared state access
- Channel operations
- Context cancellation
- Resource cleanup

### Performance Tests

**Purpose**: Validate system performance under load.

**Execution**: Docker-based performance testing

**Metrics**:
- Response times
- Throughput
- Resource utilization
- Concurrent operation limits

## Coverage Requirements

### Package-Specific Thresholds

Coverage requirements are tiered based on package criticality:

#### Critical Packages (85% minimum)
```yaml
critical_packages:
  - claude      # API integration
  - ghub        # GitHub operations
  - developer   # Core workflow logic
```

**Rationale**: These packages contain core business logic and external integrations that are critical to system functionality.

#### Infrastructure Packages (80% minimum)
```yaml
infrastructure_packages:
  - config      # Configuration management
  - state       # State persistence
  - workspace   # File operations
  - agent       # Agent lifecycle
  - orchestrator # System coordination
```

**Rationale**: Infrastructure code affects system stability and data integrity.

#### Utility Packages (75% minimum)
```yaml
utility_packages:
  - errors      # Error handling
  - observability # Logging/metrics
  - creativity  # Suggestion logic
  - gitops      # Git operations
```

**Rationale**: Utility code supports core functionality but has less direct impact on critical workflows.

### Coverage Quality Gates

Quality gates enforce minimum coverage standards:

1. **Overall Project Coverage**: ≥ 80%
2. **Package Threshold Compliance**: Each package meets its minimum
3. **Critical Path Coverage**: High-risk code paths must be tested
4. **New Code Coverage**: New code must have ≥ 80% coverage
5. **No Coverage Regression**: Coverage cannot decrease by > 2%

## Test Organization

### File Structure

```
internal/
├── package_name/
│   ├── business_logic.go
│   ├── business_logic_test.go    # Unit tests
│   ├── integration_test.go       # Package integration tests
│   └── testdata/                 # Test fixtures
└── integration/
    ├── agent_communication_test.go
    ├── workflow_handoff_test.go
    ├── shared_state_test.go
    ├── simple_agent_test.go
    ├── mock_services.go
    └── test_helpers.go
```

### Naming Conventions

- **Test Files**: `*_test.go`
- **Test Functions**: `TestFunctionName`
- **Benchmark Functions**: `BenchmarkFunctionName`
- **Example Functions**: `ExampleFunctionName`

### Test Helpers and Utilities

```go
// test_helpers.go - Common test utilities
func setupTestEnvironment(t *testing.T) *TestEnv {
    tempDir := t.TempDir()
    return &TestEnv{
        WorkspaceDir: tempDir,
        StateDir:     filepath.Join(tempDir, "state"),
    }
}

func createMockGitHubClient(t *testing.T) *MockGitHubClient {
    return &MockGitHubClient{
        // Mock configuration
    }
}
```

## Testing Workflow

### Development Workflow

1. **Write Test First**: Follow TDD when implementing new features
2. **Run Tests Locally**: `make test-unit` for quick feedback
3. **Check Coverage**: `make coverage-unit` to validate coverage
4. **Integration Testing**: `make test-integration` before committing
5. **Full Coverage Analysis**: `make coverage` for complete validation

### CI/CD Pipeline

```yaml
# Simplified workflow
on_pull_request:
  1. Run unit tests with coverage
  2. Run integration tests  
  3. Combine coverage profiles
  4. Check quality gates
  5. Generate coverage report
  6. Comment results on PR
  7. Block merge if gates fail
```

### Quality Gate Enforcement

The automated quality gates check:
- Minimum coverage thresholds
- Test execution success
- Code quality metrics
- Critical path validation

## Mock Strategy

### Mock Philosophy

Use mocks to:
- Isolate units under test
- Control external dependencies
- Simulate error conditions
- Speed up test execution

### Mock Implementations

```go
//go:generate mockery --name=GitHubClient --output=mocks
type GitHubClient interface {
    CreateIssue(ctx context.Context, issue *Issue) error
    UpdateIssue(ctx context.Context, issue *Issue) error
    GetIssue(ctx context.Context, id string) (*Issue, error)
}

// Usage in tests
func TestIssueCreation(t *testing.T) {
    mockClient := &mocks.GitHubClient{}
    mockClient.On("CreateIssue", mock.Anything, mock.Anything).Return(nil)
    
    service := NewIssueService(mockClient)
    err := service.CreateIssue(context.Background(), "test issue")
    
    assert.NoError(t, err)
    mockClient.AssertExpectations(t)
}
```

### Mock Services for Integration Tests

Integration tests use containerized mock services:
- **Mock GitHub API**: Simulates GitHub API responses
- **Mock Claude API**: Simulates Claude API interactions
- **Mock State Store**: In-memory state persistence

## Critical Path Testing

### Identified Critical Paths

1. **Workflow State Machine** (`internal/developer/workflow.go`)
   - All state transitions
   - Error handling and recovery
   - Concurrent state access

2. **API Error Handling** 
   - Network failures
   - Rate limiting
   - Authentication errors
   - Retry mechanisms

3. **State Persistence** (`internal/state/filestore.go`)
   - Data corruption scenarios
   - Concurrent access patterns
   - Recovery mechanisms

4. **Agent Communication**
   - Message delivery
   - Timeout handling
   - Concurrency control

### Critical Path Coverage Requirements

- **100% branch coverage** for error handling
- **All state transitions** must be tested
- **Failure scenarios** must be validated
- **Recovery mechanisms** must be verified

## Test Data Management

### Test Fixtures

```
internal/package/testdata/
├── valid_config.yaml
├── invalid_config.yaml
├── sample_github_response.json
└── workflow_state.json
```

### Test Data Principles

- **Minimal**: Only include necessary data
- **Realistic**: Use real-world-like test data
- **Maintainable**: Keep fixtures simple and clear
- **Isolated**: Each test uses independent data

## Performance Testing

### Performance Test Categories

1. **Load Testing**: Normal operational load
2. **Stress Testing**: Beyond normal capacity
3. **Spike Testing**: Sudden load increases
4. **Volume Testing**: Large data sets

### Performance Metrics

- **Latency**: 95th percentile response times
- **Throughput**: Requests per second
- **Resource Usage**: CPU, memory, disk I/O
- **Error Rates**: Under various load conditions

### Performance Test Environment

```bash
# Docker-based performance testing
make docker-test-performance

# Local performance profiling
go test -bench=. -cpuprofile=cpu.prof -memprofile=mem.prof
```

## Test Maintenance

### Regular Maintenance Tasks

1. **Review Coverage Reports**: Identify gaps and add tests
2. **Update Mock Expectations**: Keep mocks in sync with real APIs
3. **Refactor Test Code**: Maintain test code quality
4. **Performance Baseline Updates**: Track performance trends

### Test Debt Management

- **Flaky Test Resolution**: Fix or remove unreliable tests
- **Test Duplication**: Consolidate redundant test cases
- **Outdated Tests**: Update tests for code changes
- **Missing Coverage**: Add tests for uncovered code

## Tools and Automation

### Coverage Tools

```bash
# Generate coverage report
make coverage

# View HTML coverage
make coverage-html

# Check quality gates
make coverage-gates

# Generate coverage badge
make coverage-badge
```

### Test Automation

- **Pre-commit Hooks**: Run unit tests before commits
- **CI/CD Integration**: Full test suite on every PR
- **Nightly Builds**: Extended test suites and performance tests
- **Coverage Tracking**: Historical coverage trend analysis

## Continuous Improvement

### Metrics and Monitoring

Track these metrics over time:
- Overall test coverage percentage
- Test execution time trends
- Flaky test frequency
- Quality gate success rate

### Regular Reviews

- **Monthly**: Coverage and quality metrics review
- **Quarterly**: Testing strategy effectiveness assessment
- **Per Release**: Test suite performance and reliability analysis
- **Annual**: Comprehensive testing strategy review

### Knowledge Sharing

- **Testing Guidelines**: Document best practices
- **Code Review Focus**: Include test quality in reviews
- **Team Training**: Regular testing workshops and knowledge sharing
- **External Learning**: Stay updated with testing best practices

## Troubleshooting Guide

### Common Test Issues

1. **Tests Don't Run**
   ```bash
   # Check for build errors
   go build ./...
   
   # Verify test file naming
   ls *_test.go
   ```

2. **Coverage Not Generated**
   ```bash
   # Verify coverage flags
   go test -cover ./...
   
   # Check profile output
   go test -coverprofile=coverage.out ./...
   ```

3. **Integration Tests Fail**
   ```bash
   # Check test environment
   echo $INTEGRATION_TEST_MODE
   
   # Verify mock services
   docker compose -f docker-compose.test.yml ps
   ```

### Debug Techniques

- **Verbose Test Output**: `go test -v`
- **Specific Test Execution**: `go test -run TestName`
- **Race Detection**: `go test -race`
- **Coverage Analysis**: `go tool cover -html=coverage.out`

## Success Metrics

### Key Performance Indicators

- **Coverage**: ≥ 80% overall, package-specific thresholds met
- **Quality**: Zero critical bugs in tested code paths
- **Performance**: Test suite runs in < 10 minutes
- **Reliability**: < 1% flaky test rate
- **Developer Experience**: Easy to write and maintain tests

### Reporting and Visibility

- **PR Comments**: Automated coverage reporting
- **Dashboard**: Coverage and quality trends
- **Badges**: Visual coverage status
- **Alerts**: Quality gate failures

This comprehensive testing strategy ensures high code quality, prevents regressions, and supports confident deployment of changes to the OpenAgentFramework system.