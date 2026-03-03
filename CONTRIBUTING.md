# Contributing to DeveloperAndQAAgent

Thank you for your interest in contributing to the DeveloperAndQAAgent project! This guide outlines the requirements and processes for contributing code, documentation, and other improvements.

## Table of Contents

- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [Code Quality Requirements](#code-quality-requirements)
- [Test Coverage Requirements](#test-coverage-requirements)
- [Pull Request Process](#pull-request-process)
- [Code Style Guidelines](#code-style-guidelines)
- [Documentation Requirements](#documentation-requirements)
- [Issue Guidelines](#issue-guidelines)

## Getting Started

### Prerequisites

- Go 1.25 or later
- Git
- Make
- Docker and Docker Compose (for integration tests)
- `golangci-lint` (for code quality checks)

### Development Setup

1. **Fork and clone the repository**:
   ```bash
   git clone https://github.com/your-username/DeveloperAndQAAgent.git
   cd DeveloperAndQAAgent
   ```

2. **Install dependencies**:
   ```bash
   go mod download
   ```

3. **Verify your setup**:
   ```bash
   make build test
   ```

4. **Run coverage analysis**:
   ```bash
   make coverage
   ```

## Development Workflow

### Local Development Cycle

1. **Create a feature branch**:
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Make your changes** following the coding guidelines

3. **Run quality checks locally**:
   ```bash
   make fmt lint test
   ```

4. **Check coverage requirements**:
   ```bash
   make coverage-gates
   ```

5. **Commit your changes**:
   ```bash
   git add .
   git commit -m "feat: add your feature description"
   ```

6. **Push and create a pull request**

### Commit Message Format

Use conventional commit format:

```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `test`: Test additions or modifications
- `refactor`: Code refactoring
- `perf`: Performance improvements
- `chore`: Maintenance tasks

**Examples:**
```
feat(claude): add conversation context management
fix(ghub): handle rate limiting errors properly
docs: update coverage requirements in CONTRIBUTING.md
test(developer): add integration tests for workflow states
```

## Code Quality Requirements

All contributions must pass automated quality gates before merging:

### Quality Gates Checklist

- [ ] All tests pass (`make test`)
- [ ] Code is properly formatted (`make fmt`)
- [ ] Linting passes (`make lint`)
- [ ] Coverage requirements met (`make coverage-gates`)
- [ ] No race conditions detected
- [ ] Documentation updated (if applicable)

### Quality Standards

1. **Code Quality**:
   - Pass `golangci-lint` analysis
   - Follow Go best practices and idioms
   - Include proper error handling
   - Use meaningful variable and function names

2. **Performance**:
   - No performance regressions
   - Efficient algorithms and data structures
   - Proper resource cleanup

3. **Security**:
   - No hardcoded secrets or credentials
   - Input validation and sanitization
   - Secure API interactions

## Test Coverage Requirements

The project enforces strict test coverage standards to ensure code reliability:

### Coverage Thresholds

| Package Type | Minimum Coverage | Examples |
|--------------|------------------|----------|
| **Critical** | 85% | `claude`, `ghub`, `developer` |
| **Infrastructure** | 80% | `config`, `state`, `workspace`, `agent` |
| **Utility** | 75% | `errors`, `observability`, `creativity` |
| **Default** | 70% | All other packages |

### Coverage Requirements

1. **Overall Project Coverage**: Must maintain ≥ 80%
2. **Package Compliance**: Each package must meet its threshold
3. **New Code Coverage**: New code should have ≥ 80% coverage
4. **No Regression**: Coverage cannot decrease by more than 2%
5. **Critical Paths**: Error handling and core logic must be covered

### Coverage Best Practices

#### Unit Tests
```go
func TestWorkflowStateTransition(t *testing.T) {
    tests := []struct {
        name          string
        initialState  State
        event         Event
        expectedState State
        expectError   bool
    }{
        {
            name:          "valid idle to claim transition",
            initialState:  StateIdle,
            event:         EventClaim,
            expectedState: StateClaim,
            expectError:   false,
        },
        {
            name:         "invalid transition",
            initialState: StateIdle,
            event:        EventComplete,
            expectError:  true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            workflow := &Workflow{state: tt.initialState}
            
            err := workflow.ProcessEvent(tt.event)
            
            if tt.expectError {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tt.expectedState, workflow.state)
            }
        })
    }
}
```

#### Integration Tests
- Focus on component interactions
- Test realistic scenarios
- Use proper test fixtures and mocks
- Verify end-to-end workflows

#### Coverage Commands
```bash
# Check coverage for your changes
make coverage-unit

# Run full coverage analysis
make coverage

# Generate HTML coverage report
make coverage-html

# Validate quality gates
make coverage-gates
```

### Critical Path Testing

These areas require 100% coverage:

1. **Error Handling**: All error paths must be tested
2. **State Transitions**: Complete workflow state machine coverage
3. **API Interactions**: All GitHub and Claude API scenarios
4. **Data Persistence**: State storage and recovery mechanisms
5. **Concurrency**: Race conditions and thread safety

## Pull Request Process

### Before Creating a PR

1. **Ensure your branch is up to date**:
   ```bash
   git checkout main
   git pull upstream main
   git checkout your-branch
   git rebase main
   ```

2. **Run the full test suite**:
   ```bash
   make coverage
   ```

3. **Verify quality gates pass**:
   ```bash
   make coverage-gates
   ```

### PR Description Template

```markdown
## Description
Brief description of the changes made.

## Type of Change
- [ ] Bug fix (non-breaking change)
- [ ] New feature (non-breaking change)
- [ ] Breaking change (fix or feature causing existing functionality to change)
- [ ] Documentation update

## Testing
- [ ] Unit tests added/updated
- [ ] Integration tests added/updated
- [ ] Coverage requirements met
- [ ] Manual testing completed

## Coverage Impact
- Previous coverage: X.X%
- New coverage: X.X%
- Packages affected: list packages with coverage changes

## Checklist
- [ ] Code follows style guidelines
- [ ] Self-review completed
- [ ] Documentation updated
- [ ] Quality gates pass
- [ ] No breaking changes (or breaking changes documented)
```

### PR Review Process

1. **Automated Checks**: Quality gates must pass
2. **Coverage Analysis**: Coverage report generated automatically
3. **Code Review**: At least one approval required
4. **Documentation Review**: Ensure docs are updated
5. **Final Validation**: All checks green before merge

## Code Style Guidelines

### Go Code Style

Follow the conventions outlined in [docs/code-conventions.md](docs/code-conventions.md):

1. **Formatting**: Use `gofmt` and `goimports`
2. **Naming**: Clear, descriptive names
3. **Error Handling**: Wrap errors with context
4. **Documentation**: Public APIs must be documented
5. **Testing**: Comprehensive test coverage

### Example Code Structure

```go
// Package example demonstrates proper code organization.
package example

import (
    "context"
    "fmt"
    
    "github.com/gaskaj/DeveloperAndQAAgent/internal/state"
)

// Service represents a business service.
type Service struct {
    store state.Store
    logger *slog.Logger
}

// NewService creates a new service instance.
func NewService(store state.Store, logger *slog.Logger) *Service {
    return &Service{
        store:  store,
        logger: logger,
    }
}

// ProcessRequest handles a business request.
func (s *Service) ProcessRequest(ctx context.Context, req *Request) (*Response, error) {
    if err := req.Validate(); err != nil {
        return nil, fmt.Errorf("invalid request: %w", err)
    }
    
    // Business logic here
    
    return &Response{}, nil
}
```

## Documentation Requirements

### Required Documentation

1. **Public API Documentation**: All exported functions and types
2. **Package Documentation**: Each package needs a package comment
3. **README Updates**: For new features or significant changes
4. **Architecture Documentation**: For structural changes

### Documentation Standards

```go
// ProcessWorkflow handles the complete workflow processing pipeline.
// It validates the input, executes the workflow steps, and returns
// the final result or an error if any step fails.
//
// The workflow includes:
//   - Input validation
//   - State initialization  
//   - Step execution
//   - Result aggregation
//
// Example usage:
//   workflow := &Workflow{...}
//   result, err := ProcessWorkflow(ctx, workflow)
//   if err != nil {
//       return fmt.Errorf("workflow failed: %w", err)
//   }
func ProcessWorkflow(ctx context.Context, workflow *Workflow) (*Result, error) {
    // Implementation
}
```

## Issue Guidelines

### Creating Issues

Use the appropriate issue template and provide:

1. **Clear Description**: What needs to be done
2. **Acceptance Criteria**: How to verify completion
3. **Context**: Why this change is needed
4. **Labels**: Appropriate labels for categorization

### Issue Labels

- `bug`: Something isn't working correctly
- `enhancement`: New feature or improvement
- `documentation`: Documentation improvements
- `testing`: Test-related changes
- `agent:ready`: Ready for agent processing
- `agent:suggestion`: Agent-generated suggestions
- `priority:high`: High priority items

## Getting Help

### Resources

- [Architecture Documentation](docs/architecture.md)
- [Developer Workflow](docs/developer-workflow.md)
- [Test Coverage Guide](docs/test-coverage.md)
- [Quality Assurance](docs/quality-assurance.md)

### Support Channels

1. **GitHub Issues**: For bugs and feature requests
2. **GitHub Discussions**: For questions and general discussion
3. **Code Reviews**: For implementation guidance
4. **Documentation**: Check existing docs first

### Debugging and Troubleshooting

#### Common Issues

1. **Tests Failing**:
   ```bash
   # Run specific test
   go test -v ./internal/package -run TestName
   
   # Check for race conditions
   go test -race ./...
   ```

2. **Coverage Issues**:
   ```bash
   # Generate detailed coverage report
   make coverage-html
   # Open coverage.html in browser
   ```

3. **Quality Gate Failures**:
   ```bash
   # Check specific quality gate
   make coverage-gates
   
   # Run full analysis
   ./scripts/coverage.sh analyze
   ```

## Advanced Topics

### Integration Testing

Integration tests are located in `./internal/integration/` and test:

- Agent communication protocols
- Workflow handoff mechanisms  
- Shared state management
- End-to-end scenarios

Run integration tests:
```bash
make test-integration
```

### Performance Testing

Performance tests validate:
- Response times under load
- Resource utilization
- Concurrent operation limits

Run performance tests:
```bash
make docker-test-performance
```

### Security Considerations

- Never commit API keys or secrets
- Validate all external inputs
- Use secure communication protocols
- Follow principle of least privilege

## Release Process

### Versioning

The project follows semantic versioning:
- Major: Breaking changes
- Minor: New features (backward compatible)
- Patch: Bug fixes (backward compatible)

### Release Checklist

1. All tests pass
2. Coverage requirements met
3. Documentation updated
4. CHANGELOG updated
5. Version tagged

## Contributing License

By contributing to this project, you agree that your contributions will be licensed under the same license as the project.

## Recognition

Contributors are recognized in:
- Commit history
- Release notes
- Project documentation

Thank you for contributing to making the DeveloperAndQAAgent project better!