# Test Coverage Documentation

This document describes the comprehensive test coverage framework implemented for the DeveloperAndQAAgent project, including coverage collection, analysis, reporting, and quality gates.

## Overview

The coverage framework provides:
- **Comprehensive Coverage Collection**: Unit and integration test coverage
- **Package-Level Analysis**: Per-package coverage with specific thresholds
- **Quality Gates**: Automated enforcement of coverage standards
- **Detailed Reporting**: HTML, markdown, and JSON coverage reports
- **CI/CD Integration**: Automated coverage analysis in pull requests
- **Coverage Badges**: Visual coverage status in README

## Coverage Architecture

### Coverage Collection Pipeline

```
Unit Tests (short)     Integration Tests
       ↓                        ↓
coverage-unit.out         coverage-integration.out
       ↓                        ↓
       └─────── Combine ────────┘
                  ↓
         coverage-combined.out
                  ↓
      ┌─────────────────────────┐
      │  Analysis & Reporting   │
      ├─────────────────────────┤
      │ • HTML Report           │
      │ • Function Report       │
      │ • Package Analysis      │
      │ • Quality Gates         │
      │ • Coverage Badge        │
      └─────────────────────────┘
```

### Files and Scripts

| File | Purpose |
|------|---------|
| `scripts/coverage.sh` | Main coverage collection and analysis script |
| `scripts/coverage-report.go` | Detailed coverage report generator |
| `scripts/generate-coverage-badge.sh` | Coverage badge generation |
| `internal/testing/coverage.go` | Coverage analysis utilities |
| `configs/quality-gates.yaml` | Coverage thresholds and quality rules |
| `.github/workflows/quality-gates.yml` | CI/CD quality gate automation |

## Package Coverage Thresholds

The project uses differentiated coverage thresholds based on package criticality:

### Critical Packages (85% minimum)
- **claude**: Claude API integration and conversation management
- **ghub**: GitHub API client and operations  
- **developer**: Developer agent workflow and state management

### Infrastructure Packages (80% minimum)
- **config**: Configuration management and validation
- **state**: State persistence and recovery
- **workspace**: Workspace management and file operations
- **agent**: Agent lifecycle and coordination
- **orchestrator**: System orchestration and health management

### Utility Packages (75% minimum)
- **errors**: Error handling and retry logic
- **observability**: Logging, metrics, and monitoring
- **creativity**: Creative suggestion and context management
- **gitops**: Git operations and repository management

### Default Threshold
- **All other packages**: 70% minimum

## Usage

### Running Coverage Analysis

#### Complete Coverage Analysis
```bash
make coverage
# Runs unit tests, integration tests, combines coverage, generates reports and badges
```

#### Individual Coverage Tasks
```bash
# Unit test coverage only
make coverage-unit

# Integration test coverage only  
make coverage-integration

# Generate HTML report
make coverage-html

# Generate coverage badge
make coverage-badge

# Check quality gates
make coverage-gates
```

#### Using the Coverage Script
```bash
# Run all coverage analysis
./scripts/coverage.sh all

# Run only unit tests
./scripts/coverage.sh unit

# Run only integration tests
./scripts/coverage.sh integration

# Analyze existing coverage data
./scripts/coverage.sh analyze
```

### Coverage Report Generator

The `scripts/coverage-report.go` utility provides detailed coverage analysis:

```bash
# Generate markdown report
go run scripts/coverage-report.go -profile=coverage-combined.out -format=markdown -output=report.md

# Check quality gates
go run scripts/coverage-report.go -profile=coverage-combined.out -check-gates -verbose

# Generate JSON report
go run scripts/coverage-report.go -profile=coverage-combined.out -format=json | jq '.overall_coverage'
```

## Quality Gates

Quality gates are automated checks that enforce coverage standards:

### Gate Rules
1. **Overall Coverage**: Must be ≥ 80%
2. **Package Thresholds**: Each package must meet its specific threshold
3. **Critical Paths**: High-risk code paths must be covered
4. **New Code**: New code must have ≥ 80% coverage (when detectable)

### Quality Gate Workflow
1. Tests run with coverage collection
2. Coverage profiles are combined and analyzed
3. Package-level coverage is calculated
4. Thresholds are checked against `configs/quality-gates.yaml`
5. Gates pass/fail based on all criteria
6. PR is blocked if gates fail

### Configuration

Coverage thresholds and rules are defined in `configs/quality-gates.yaml`:

```yaml
coverage:
  overall_minimum: 80
  package_thresholds:
    claude: 85
    ghub: 85
    developer: 85
    # ... more packages
    
quality_gates:
  enforce_minimum_coverage: true
  enforce_package_thresholds: true
  max_coverage_decrease: 2.0
```

## Reports and Output

### Generated Reports

1. **HTML Coverage Report**: `coverage.html`
   - Interactive coverage visualization
   - File-by-file coverage details
   - Clickable source code with coverage highlighting

2. **Function Coverage Report**: `coverage-func.txt`
   - Function-level coverage percentages
   - Package summaries
   - Overall coverage statistics

3. **Package Analysis Report**: `package-coverage-report.md`
   - Per-package coverage breakdown
   - Threshold compliance status
   - Critical uncovered paths

4. **Detailed Coverage Report**: `coverage-report.md`
   - Comprehensive analysis
   - Quality gate status
   - Recommendations for improvement

### Coverage Badge

The coverage badge is automatically generated and can be included in README:

```markdown
![Coverage](https://img.shields.io/badge/coverage-85%25-brightgreen)
```

Badge colors:
- **Green (≥80%)**: Excellent coverage
- **Yellow (≥70%)**: Good coverage  
- **Orange (≥60%)**: Acceptable coverage
- **Red (<60%)**: Poor coverage

## CI/CD Integration

### GitHub Actions Workflow

The `.github/workflows/quality-gates.yml` workflow:
1. Runs on all PRs and pushes to main/develop
2. Executes unit and integration tests with coverage
3. Combines coverage profiles
4. Generates detailed reports
5. Checks quality gates
6. Comments coverage status on PRs
7. Uploads coverage artifacts
8. Fails the build if quality gates don't pass

### PR Coverage Comments

Pull requests automatically receive coverage comments with:
- Overall coverage percentage
- Coverage badge
- Per-package coverage status
- Quality gate results
- Links to detailed reports

## Best Practices

### Writing Coverage-Friendly Tests

1. **Test Public APIs**: Focus on exported functions and methods
2. **Cover Error Paths**: Ensure error handling is tested
3. **Test Edge Cases**: Include boundary conditions and corner cases
4. **Use Table-Driven Tests**: Efficient way to cover multiple scenarios

```go
func TestWorkflowTransitions(t *testing.T) {
    tests := []struct {
        name        string
        currentState State
        event       Event
        expectedState State
        shouldError bool
    }{
        {
            name:         "valid transition",
            currentState: StateIdle,
            event:        EventStart,
            expectedState: StateRunning,
            shouldError:  false,
        },
        // ... more test cases
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

### Improving Coverage

1. **Identify Uncovered Lines**: Use HTML report to find missed code
2. **Focus on Critical Packages**: Prioritize high-threshold packages
3. **Test Error Conditions**: Many uncovered lines are error paths
4. **Add Integration Tests**: For API clients and system interactions

### Coverage Exemptions

Some code may be exempt from coverage requirements:
- Generated code
- Test utilities and mocks
- Vendor code
- Example/demo code

Configure exemptions in `configs/quality-gates.yaml`.

## Troubleshooting

### Common Issues

1. **"No coverage data found"**
   - Ensure tests are actually running
   - Check for build errors preventing test execution
   - Verify coverage profile paths are correct

2. **"Quality gates failed"**
   - Check which packages are below threshold
   - Review uncovered code paths in HTML report
   - Add tests for critical missing coverage

3. **"Integration tests not running"**
   - Verify `./internal/integration/` directory exists
   - Check that `INTEGRATION_TEST_MODE` environment variable is set
   - Ensure test services are running (if required)

### Debug Commands

```bash
# Verbose coverage analysis
./scripts/coverage.sh all | tee coverage-debug.log

# Check specific package coverage
go tool cover -func=coverage-combined.out | grep "package_name"

# Validate coverage profile
go tool cover -func=coverage-combined.out | head -10
```

### Performance Considerations

- **Parallel Test Execution**: Enabled by default for faster coverage collection
- **Coverage Mode**: Uses `atomic` mode for accurate concurrent coverage
- **Timeout Limits**: Integration tests have 30-minute timeout
- **Artifact Cleanup**: Old coverage files are cleaned automatically

## Future Enhancements

Potential improvements to the coverage system:

1. **Coverage Trends**: Track coverage changes over time
2. **Differential Coverage**: Show coverage for only changed code
3. **Critical Path Detection**: Automatically identify high-risk code paths
4. **Coverage Heatmaps**: Visual representation of coverage across packages
5. **Performance Impact**: Measure test performance impact of coverage collection

## References

- [Go Coverage Documentation](https://golang.org/doc/code/coverage)
- [Testing in Go](https://golang.org/doc/code/testing)
- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [Shields.io Badge API](https://shields.io/)

## Support

For questions about the coverage system:
1. Review this documentation
2. Check existing issues in the project repository
3. Create a new issue with the `testing` label
4. Include relevant log output and configuration details