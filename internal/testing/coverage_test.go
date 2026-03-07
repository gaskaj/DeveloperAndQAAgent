package testing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCoverageAnalyzer(t *testing.T) {
	ca := NewCoverageAnalyzer()
	require.NotNil(t, ca)
	assert.NotNil(t, ca.profiles)
	assert.NotNil(t, ca.packageStats)
	assert.NotNil(t, ca.thresholds)

	// Check default thresholds
	assert.Equal(t, 85.0, ca.thresholds["claude"])
	assert.Equal(t, 85.0, ca.thresholds["ghub"])
	assert.Equal(t, 85.0, ca.thresholds["developer"])
	assert.Equal(t, 80.0, ca.thresholds["config"])
	assert.Equal(t, 80.0, ca.thresholds["state"])
	assert.Equal(t, 75.0, ca.thresholds["errors"])
	assert.Equal(t, 75.0, ca.thresholds["observability"])
}

func TestLoadCoverageProfile(t *testing.T) {
	t.Run("valid profile", func(t *testing.T) {
		tmpDir := t.TempDir()
		profilePath := filepath.Join(tmpDir, "coverage.out")
		content := `mode: set
github.com/gaskaj/OpenAgentFramework/internal/config/config.go:10.30,12.2 1 1
github.com/gaskaj/OpenAgentFramework/internal/config/config.go:14.30,16.2 1 0
github.com/gaskaj/OpenAgentFramework/internal/errors/retry.go:20.40,25.2 3 1
`
		require.NoError(t, os.WriteFile(profilePath, []byte(content), 0o644))

		ca := NewCoverageAnalyzer()
		err := ca.LoadCoverageProfile(profilePath)
		require.NoError(t, err)
		assert.Len(t, ca.profiles, 3)
	})

	t.Run("file not found", func(t *testing.T) {
		ca := NewCoverageAnalyzer()
		err := ca.LoadCoverageProfile("/nonexistent/coverage.out")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open coverage profile")
	})

	t.Run("invalid line format", func(t *testing.T) {
		tmpDir := t.TempDir()
		profilePath := filepath.Join(tmpDir, "coverage.out")
		content := "mode: set\ninvalid line\n"
		require.NoError(t, os.WriteFile(profilePath, []byte(content), 0o644))

		ca := NewCoverageAnalyzer()
		err := ca.LoadCoverageProfile(profilePath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse line")
	})

	t.Run("empty lines skipped", func(t *testing.T) {
		tmpDir := t.TempDir()
		profilePath := filepath.Join(tmpDir, "coverage.out")
		content := "mode: set\n\n\n"
		require.NoError(t, os.WriteFile(profilePath, []byte(content), 0o644))

		ca := NewCoverageAnalyzer()
		err := ca.LoadCoverageProfile(profilePath)
		require.NoError(t, err)
		assert.Empty(t, ca.profiles)
	})
}

func TestParseCoverageLine(t *testing.T) {
	ca := NewCoverageAnalyzer()

	t.Run("valid line", func(t *testing.T) {
		profile, err := ca.parseCoverageLine("github.com/pkg/file.go:10.5,20.10 3 1")
		require.NoError(t, err)
		assert.Equal(t, "github.com/pkg/file.go", profile.FileName)
		assert.Equal(t, 10, profile.StartLine)
		assert.Equal(t, 5, profile.StartCol)
		assert.Equal(t, 20, profile.EndLine)
		assert.Equal(t, 10, profile.EndCol)
		assert.Equal(t, 3, profile.NumStmt)
		assert.Equal(t, 1, profile.Count)
	})

	t.Run("wrong number of fields", func(t *testing.T) {
		_, err := ca.parseCoverageLine("onlyone")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid coverage line format")
	})

	t.Run("missing colon", func(t *testing.T) {
		_, err := ca.parseCoverageLine("nofilecolon 1 2")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid file:position format")
	})

	t.Run("missing comma in position", func(t *testing.T) {
		_, err := ca.parseCoverageLine("file.go:10.5 1 2")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid position format")
	})

	t.Run("invalid start parts", func(t *testing.T) {
		_, err := ca.parseCoverageLine("file.go:10,20.5 1 2")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid position parts")
	})

	t.Run("invalid start line number", func(t *testing.T) {
		_, err := ca.parseCoverageLine("file.go:abc.5,20.10 1 2")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid start line")
	})

	t.Run("invalid start column", func(t *testing.T) {
		_, err := ca.parseCoverageLine("file.go:10.abc,20.10 1 2")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid start column")
	})

	t.Run("invalid end line", func(t *testing.T) {
		_, err := ca.parseCoverageLine("file.go:10.5,abc.10 1 2")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid end line")
	})

	t.Run("invalid end column", func(t *testing.T) {
		_, err := ca.parseCoverageLine("file.go:10.5,20.abc 1 2")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid end column")
	})

	t.Run("invalid statement count", func(t *testing.T) {
		_, err := ca.parseCoverageLine("file.go:10.5,20.10 abc 2")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid statement count")
	})

	t.Run("invalid execution count", func(t *testing.T) {
		_, err := ca.parseCoverageLine("file.go:10.5,20.10 1 abc")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid execution count")
	})
}

func TestExtractPackageName(t *testing.T) {
	ca := NewCoverageAnalyzer()

	tests := []struct {
		filePath string
		expected string
	}{
		{"github.com/gaskaj/OpenAgentFramework/internal/config/config.go", "config"},
		{"github.com/gaskaj/OpenAgentFramework/internal/errors/retry.go", "errors"},
		{"github.com/gaskaj/OpenAgentFramework/internal/developer/workflow.go", "developer"},
		{"some/other/path/file.go", "path"},
	}

	for _, tt := range tests {
		t.Run(tt.filePath, func(t *testing.T) {
			result := ca.extractPackageName(tt.filePath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculateFileStats(t *testing.T) {
	ca := NewCoverageAnalyzer()

	profiles := []CoverageProfile{
		{FileName: "test.go", StartLine: 10, EndLine: 12, NumStmt: 3, Count: 1},
		{FileName: "test.go", StartLine: 15, EndLine: 17, NumStmt: 2, Count: 0},
		{FileName: "test.go", StartLine: 20, EndLine: 20, NumStmt: 1, Count: 5},
	}

	stats := ca.calculateFileStats("test.go", profiles)
	assert.Equal(t, "test.go", stats.FileName)
	assert.Equal(t, 6, stats.TotalStatements)
	assert.Equal(t, 4, stats.CoveredStmts)
	assert.InDelta(t, 66.67, stats.CoveragePercent, 0.1)
	assert.Contains(t, stats.UncoveredLines, 15)
	assert.Contains(t, stats.UncoveredLines, 16)
	assert.Contains(t, stats.UncoveredLines, 17)
}

func TestCalculateFileStatsEmpty(t *testing.T) {
	ca := NewCoverageAnalyzer()
	stats := ca.calculateFileStats("empty.go", nil)
	assert.Equal(t, 0, stats.TotalStatements)
	assert.Equal(t, 0.0, stats.CoveragePercent)
}

func TestDeduplicateInts(t *testing.T) {
	ca := NewCoverageAnalyzer()

	tests := []struct {
		name     string
		input    []int
		expected []int
	}{
		{"empty", []int{}, []int{}},
		{"single", []int{1}, []int{1}},
		{"no duplicates", []int{1, 2, 3}, []int{1, 2, 3}},
		{"with duplicates", []int{1, 1, 2, 2, 3, 3, 3}, []int{1, 2, 3}},
		{"all same", []int{5, 5, 5}, []int{5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ca.deduplicateInts(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetPackageStats(t *testing.T) {
	ca := NewCoverageAnalyzer()
	ca.packageStats["test"] = &PackageCoverage{
		PackageName:     "test",
		TotalStatements: 100,
		CoveredStmts:    80,
		CoveragePercent: 80.0,
	}

	stats := ca.GetPackageStats()
	require.NotNil(t, stats)
	assert.Contains(t, stats, "test")
	assert.Equal(t, 80.0, stats["test"].CoveragePercent)
}

func TestGetOverallCoverage(t *testing.T) {
	t.Run("with data", func(t *testing.T) {
		ca := NewCoverageAnalyzer()
		ca.packageStats["pkg1"] = &PackageCoverage{TotalStatements: 100, CoveredStmts: 80}
		ca.packageStats["pkg2"] = &PackageCoverage{TotalStatements: 100, CoveredStmts: 60}

		overall := ca.GetOverallCoverage()
		assert.InDelta(t, 70.0, overall, 0.1)
	})

	t.Run("empty", func(t *testing.T) {
		ca := NewCoverageAnalyzer()
		assert.Equal(t, 0.0, ca.GetOverallCoverage())
	})
}

func TestCheckThresholds(t *testing.T) {
	ca := NewCoverageAnalyzer()
	ca.packageStats["config"] = &PackageCoverage{CoveragePercent: 85.0}
	ca.packageStats["errors"] = &PackageCoverage{CoveragePercent: 50.0}
	ca.packageStats["unknown"] = &PackageCoverage{CoveragePercent: 75.0}

	results := ca.CheckThresholds()
	assert.True(t, results["config"])  // 85% >= 80% threshold
	assert.False(t, results["errors"]) // 50% < 75% threshold
	assert.True(t, results["unknown"]) // 75% >= 70% default
}

func TestGetThreshold(t *testing.T) {
	ca := NewCoverageAnalyzer()
	assert.Equal(t, 85.0, ca.GetThreshold("claude"))
	assert.Equal(t, 80.0, ca.GetThreshold("config"))
	assert.Equal(t, 75.0, ca.GetThreshold("errors"))
	assert.Equal(t, 70.0, ca.GetThreshold("unknown_package"))
}

func TestSetThreshold(t *testing.T) {
	ca := NewCoverageAnalyzer()
	ca.SetThreshold("custom", 90.0)
	assert.Equal(t, 90.0, ca.GetThreshold("custom"))

	ca.SetThreshold("config", 95.0)
	assert.Equal(t, 95.0, ca.GetThreshold("config"))
}

func TestFindCriticalUncoveredPaths(t *testing.T) {
	ca := NewCoverageAnalyzer()
	ca.packageStats["config"] = &PackageCoverage{
		PackageName:     "config",
		CoveragePercent: 70.0,
		Files: map[string]*FileCoverage{
			"config.go": {
				FileName:        "config.go",
				CoveragePercent: 60.0,
				UncoveredLines:  []int{10, 11, 12},
			},
		},
	}
	ca.packageStats["errors"] = &PackageCoverage{
		PackageName:     "errors",
		CoveragePercent: 90.0,
		Files: map[string]*FileCoverage{
			"retry.go": {
				FileName:        "retry.go",
				CoveragePercent: 90.0,
				UncoveredLines:  []int{5},
			},
		},
	}

	critical := ca.FindCriticalUncoveredPaths()
	assert.Contains(t, critical, "config")    // threshold is 80%, coverage is 60% for file
	assert.NotContains(t, critical, "errors") // threshold is 75%, below 80 cutoff
}

func TestGenerateReport(t *testing.T) {
	ca := NewCoverageAnalyzer()
	ca.packageStats["config"] = &PackageCoverage{
		PackageName:     "config",
		TotalStatements: 100,
		CoveredStmts:    85,
		CoveragePercent: 85.0,
		Files:           make(map[string]*FileCoverage),
	}
	ca.packageStats["errors"] = &PackageCoverage{
		PackageName:     "errors",
		TotalStatements: 50,
		CoveredStmts:    40,
		CoveragePercent: 80.0,
		Files:           make(map[string]*FileCoverage),
	}

	report := ca.GenerateReport()
	assert.Contains(t, report, "Test Coverage Report")
	assert.Contains(t, report, "Overall Coverage")
	assert.Contains(t, report, "Package Coverage Breakdown")
	assert.Contains(t, report, "config")
	assert.Contains(t, report, "errors")
}

func TestCalculatePackageStats(t *testing.T) {
	ca := NewCoverageAnalyzer()
	ca.profiles = []CoverageProfile{
		{FileName: "github.com/gaskaj/OpenAgentFramework/internal/config/config.go", StartLine: 10, EndLine: 12, NumStmt: 3, Count: 1},
		{FileName: "github.com/gaskaj/OpenAgentFramework/internal/config/validate.go", StartLine: 5, EndLine: 7, NumStmt: 2, Count: 0},
		{FileName: "github.com/gaskaj/OpenAgentFramework/internal/errors/retry.go", StartLine: 10, EndLine: 15, NumStmt: 5, Count: 3},
	}

	err := ca.calculatePackageStats()
	require.NoError(t, err)
	assert.Contains(t, ca.packageStats, "config")
	assert.Contains(t, ca.packageStats, "errors")

	configStats := ca.packageStats["config"]
	assert.Equal(t, 5, configStats.TotalStatements)
	assert.Equal(t, 3, configStats.CoveredStmts)

	errStats := ca.packageStats["errors"]
	assert.Equal(t, 5, errStats.TotalStatements)
	assert.Equal(t, 5, errStats.CoveredStmts)
}

func TestEndToEnd(t *testing.T) {
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "coverage.out")

	content := `mode: set
github.com/gaskaj/OpenAgentFramework/internal/config/config.go:10.30,12.2 3 1
github.com/gaskaj/OpenAgentFramework/internal/config/config.go:14.30,18.2 5 0
github.com/gaskaj/OpenAgentFramework/internal/config/validate.go:5.20,8.2 2 1
github.com/gaskaj/OpenAgentFramework/internal/errors/retry.go:10.30,15.2 5 1
github.com/gaskaj/OpenAgentFramework/internal/errors/retry.go:20.30,25.2 3 0
`
	require.NoError(t, os.WriteFile(profilePath, []byte(content), 0o644))

	ca := NewCoverageAnalyzer()
	require.NoError(t, ca.LoadCoverageProfile(profilePath))

	// Check package stats
	stats := ca.GetPackageStats()
	require.Contains(t, stats, "config")
	require.Contains(t, stats, "errors")

	// Check overall coverage
	overall := ca.GetOverallCoverage()
	assert.True(t, overall > 0)

	// Check thresholds
	thresholds := ca.CheckThresholds()
	assert.Contains(t, thresholds, "config")
	assert.Contains(t, thresholds, "errors")

	// Generate report
	report := ca.GenerateReport()
	assert.True(t, len(report) > 0)
	assert.True(t, strings.Contains(report, "config"))
}
