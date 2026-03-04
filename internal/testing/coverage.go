// Package testing provides utilities for test coverage analysis and reporting.
package testing

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// CoverageProfile represents a single line in a Go coverage profile.
type CoverageProfile struct {
	FileName  string
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
	NumStmt   int
	Count     int
}

// PackageCoverage represents coverage statistics for a package.
type PackageCoverage struct {
	PackageName     string
	TotalStatements int
	CoveredStmts    int
	CoveragePercent float64
	Files           map[string]*FileCoverage
}

// FileCoverage represents coverage statistics for a single file.
type FileCoverage struct {
	FileName        string
	TotalStatements int
	CoveredStmts    int
	CoveragePercent float64
	UncoveredLines  []int
}

// CoverageAnalyzer provides methods for analyzing test coverage data.
type CoverageAnalyzer struct {
	profiles     []CoverageProfile
	packageStats map[string]*PackageCoverage
	thresholds   map[string]float64
}

// NewCoverageAnalyzer creates a new coverage analyzer.
func NewCoverageAnalyzer() *CoverageAnalyzer {
	return &CoverageAnalyzer{
		profiles:     make([]CoverageProfile, 0),
		packageStats: make(map[string]*PackageCoverage),
		thresholds: map[string]float64{
			// Critical packages requiring high coverage
			"claude":    85.0,
			"ghub":      85.0,
			"developer": 85.0,
			// Infrastructure packages
			"config":       80.0,
			"state":        80.0,
			"workspace":    80.0,
			"agent":        80.0,
			"orchestrator": 80.0,
			// Utility packages
			"errors":        75.0,
			"observability": 75.0,
			"creativity":    75.0,
			"gitops":        75.0,
		},
	}
}

// LoadCoverageProfile loads coverage data from a Go coverage profile file.
func (ca *CoverageAnalyzer) LoadCoverageProfile(profilePath string) error {
	file, err := os.Open(profilePath)
	if err != nil {
		return fmt.Errorf("failed to open coverage profile: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		lineNum++

		// Skip mode line and empty lines
		if lineNum == 1 || line == "" {
			continue
		}

		profile, err := ca.parseCoverageLine(line)
		if err != nil {
			return fmt.Errorf("failed to parse line %d: %w", lineNum, err)
		}

		ca.profiles = append(ca.profiles, profile)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading coverage profile: %w", err)
	}

	return ca.calculatePackageStats()
}

// parseCoverageLine parses a single line from a coverage profile.
func (ca *CoverageAnalyzer) parseCoverageLine(line string) (CoverageProfile, error) {
	// Format: filename.go:startLine.startCol,endLine.endCol numStmt count
	parts := strings.Fields(line)
	if len(parts) != 3 {
		return CoverageProfile{}, fmt.Errorf("invalid coverage line format: %s", line)
	}

	// Parse file and position
	filePart := parts[0]
	colonIndex := strings.LastIndex(filePart, ":")
	if colonIndex == -1 {
		return CoverageProfile{}, fmt.Errorf("invalid file:position format: %s", filePart)
	}

	filename := filePart[:colonIndex]
	position := filePart[colonIndex+1:]

	// Parse position (startLine.startCol,endLine.endCol)
	commaIndex := strings.Index(position, ",")
	if commaIndex == -1 {
		return CoverageProfile{}, fmt.Errorf("invalid position format: %s", position)
	}

	startPos := position[:commaIndex]
	endPos := position[commaIndex+1:]

	startParts := strings.Split(startPos, ".")
	endParts := strings.Split(endPos, ".")

	if len(startParts) != 2 || len(endParts) != 2 {
		return CoverageProfile{}, fmt.Errorf("invalid position parts: %s", position)
	}

	startLine, err := strconv.Atoi(startParts[0])
	if err != nil {
		return CoverageProfile{}, fmt.Errorf("invalid start line: %w", err)
	}

	startCol, err := strconv.Atoi(startParts[1])
	if err != nil {
		return CoverageProfile{}, fmt.Errorf("invalid start column: %w", err)
	}

	endLine, err := strconv.Atoi(endParts[0])
	if err != nil {
		return CoverageProfile{}, fmt.Errorf("invalid end line: %w", err)
	}

	endCol, err := strconv.Atoi(endParts[1])
	if err != nil {
		return CoverageProfile{}, fmt.Errorf("invalid end column: %w", err)
	}

	// Parse statement count and execution count
	numStmt, err := strconv.Atoi(parts[1])
	if err != nil {
		return CoverageProfile{}, fmt.Errorf("invalid statement count: %w", err)
	}

	count, err := strconv.Atoi(parts[2])
	if err != nil {
		return CoverageProfile{}, fmt.Errorf("invalid execution count: %w", err)
	}

	return CoverageProfile{
		FileName:  filename,
		StartLine: startLine,
		StartCol:  startCol,
		EndLine:   endLine,
		EndCol:    endCol,
		NumStmt:   numStmt,
		Count:     count,
	}, nil
}

// calculatePackageStats calculates coverage statistics for each package.
func (ca *CoverageAnalyzer) calculatePackageStats() error {
	// Group profiles by package
	packageProfiles := make(map[string][]CoverageProfile)

	for _, profile := range ca.profiles {
		// Extract package name from file path
		packageName := ca.extractPackageName(profile.FileName)
		packageProfiles[packageName] = append(packageProfiles[packageName], profile)
	}

	// Calculate stats for each package
	for packageName, profiles := range packageProfiles {
		stats := &PackageCoverage{
			PackageName: packageName,
			Files:       make(map[string]*FileCoverage),
		}

		// Group by files within package
		fileProfiles := make(map[string][]CoverageProfile)
		for _, profile := range profiles {
			fileProfiles[profile.FileName] = append(fileProfiles[profile.FileName], profile)
		}

		// Calculate file-level stats
		for fileName, fProfiles := range fileProfiles {
			fileStat := ca.calculateFileStats(fileName, fProfiles)
			stats.Files[fileName] = fileStat
			stats.TotalStatements += fileStat.TotalStatements
			stats.CoveredStmts += fileStat.CoveredStmts
		}

		// Calculate package coverage percentage
		if stats.TotalStatements > 0 {
			stats.CoveragePercent = float64(stats.CoveredStmts) / float64(stats.TotalStatements) * 100
		}

		ca.packageStats[packageName] = stats
	}

	return nil
}

// extractPackageName extracts the package name from a file path.
func (ca *CoverageAnalyzer) extractPackageName(filePath string) string {
	// Handle module-relative paths
	if strings.Contains(filePath, "/internal/") {
		parts := strings.Split(filePath, "/internal/")
		if len(parts) > 1 {
			internalPath := parts[1]
			pathParts := strings.Split(internalPath, "/")
			if len(pathParts) > 0 {
				return pathParts[0]
			}
		}
	}

	// Fallback to directory name
	return filepath.Base(filepath.Dir(filePath))
}

// calculateFileStats calculates coverage statistics for a single file.
func (ca *CoverageAnalyzer) calculateFileStats(fileName string, profiles []CoverageProfile) *FileCoverage {
	fileStat := &FileCoverage{
		FileName:       fileName,
		UncoveredLines: make([]int, 0),
	}

	for _, profile := range profiles {
		fileStat.TotalStatements += profile.NumStmt
		if profile.Count > 0 {
			fileStat.CoveredStmts += profile.NumStmt
		} else {
			// Add uncovered lines
			for line := profile.StartLine; line <= profile.EndLine; line++ {
				fileStat.UncoveredLines = append(fileStat.UncoveredLines, line)
			}
		}
	}

	// Calculate file coverage percentage
	if fileStat.TotalStatements > 0 {
		fileStat.CoveragePercent = float64(fileStat.CoveredStmts) / float64(fileStat.TotalStatements) * 100
	}

	// Sort and deduplicate uncovered lines
	sort.Ints(fileStat.UncoveredLines)
	fileStat.UncoveredLines = ca.deduplicateInts(fileStat.UncoveredLines)

	return fileStat
}

// deduplicateInts removes duplicate integers from a sorted slice.
func (ca *CoverageAnalyzer) deduplicateInts(nums []int) []int {
	if len(nums) == 0 {
		return nums
	}

	result := make([]int, 0, len(nums))
	prev := nums[0]
	result = append(result, prev)

	for i := 1; i < len(nums); i++ {
		if nums[i] != prev {
			result = append(result, nums[i])
			prev = nums[i]
		}
	}

	return result
}

// GetPackageStats returns coverage statistics for all packages.
func (ca *CoverageAnalyzer) GetPackageStats() map[string]*PackageCoverage {
	return ca.packageStats
}

// GetOverallCoverage calculates the overall coverage percentage.
func (ca *CoverageAnalyzer) GetOverallCoverage() float64 {
	totalStmts := 0
	coveredStmts := 0

	for _, stats := range ca.packageStats {
		totalStmts += stats.TotalStatements
		coveredStmts += stats.CoveredStmts
	}

	if totalStmts == 0 {
		return 0.0
	}

	return float64(coveredStmts) / float64(totalStmts) * 100
}

// CheckThresholds checks if packages meet their coverage thresholds.
func (ca *CoverageAnalyzer) CheckThresholds() map[string]bool {
	results := make(map[string]bool)

	for packageName, stats := range ca.packageStats {
		threshold, exists := ca.thresholds[packageName]
		if !exists {
			threshold = 70.0 // Default threshold
		}

		results[packageName] = stats.CoveragePercent >= threshold
	}

	return results
}

// GetThreshold returns the coverage threshold for a package.
func (ca *CoverageAnalyzer) GetThreshold(packageName string) float64 {
	if threshold, exists := ca.thresholds[packageName]; exists {
		return threshold
	}
	return 70.0 // Default threshold
}

// SetThreshold sets the coverage threshold for a package.
func (ca *CoverageAnalyzer) SetThreshold(packageName string, threshold float64) {
	ca.thresholds[packageName] = threshold
}

// FindCriticalUncoveredPaths identifies uncovered code in critical packages.
func (ca *CoverageAnalyzer) FindCriticalUncoveredPaths() map[string][]string {
	critical := make(map[string][]string)

	for packageName, stats := range ca.packageStats {
		threshold := ca.GetThreshold(packageName)

		// Consider packages with thresholds >= 80% as critical
		if threshold >= 80.0 {
			var uncoveredPaths []string

			for fileName, fileStat := range stats.Files {
				if len(fileStat.UncoveredLines) > 0 && fileStat.CoveragePercent < threshold {
					uncoveredPath := fmt.Sprintf("%s (lines: %v, coverage: %.1f%%)",
						fileName, fileStat.UncoveredLines, fileStat.CoveragePercent)
					uncoveredPaths = append(uncoveredPaths, uncoveredPath)
				}
			}

			if len(uncoveredPaths) > 0 {
				critical[packageName] = uncoveredPaths
			}
		}
	}

	return critical
}

// GenerateReport generates a comprehensive coverage report.
func (ca *CoverageAnalyzer) GenerateReport() string {
	var report strings.Builder

	report.WriteString("# Test Coverage Report\n\n")
	report.WriteString(fmt.Sprintf("Generated on: %s\n\n", strings.Split(fmt.Sprintf("%v", os.Getenv("BUILD_TIME")), " ")[0]))

	// Overall coverage
	overall := ca.GetOverallCoverage()
	report.WriteString(fmt.Sprintf("## Overall Coverage: %.2f%%\n\n", overall))

	// Package breakdown
	report.WriteString("## Package Coverage Breakdown\n\n")
	report.WriteString("| Package | Coverage | Threshold | Status |\n")
	report.WriteString("|---------|----------|-----------|--------|\n")

	// Sort packages by name for consistent output
	var packageNames []string
	for name := range ca.packageStats {
		packageNames = append(packageNames, name)
	}
	sort.Strings(packageNames)

	for _, packageName := range packageNames {
		stats := ca.packageStats[packageName]
		threshold := ca.GetThreshold(packageName)
		status := "✓ PASS"
		if stats.CoveragePercent < threshold {
			status = "✗ FAIL"
		}

		report.WriteString(fmt.Sprintf("| %s | %.2f%% | %.1f%% | %s |\n",
			packageName, stats.CoveragePercent, threshold, status))
	}

	// Critical uncovered paths
	critical := ca.FindCriticalUncoveredPaths()
	if len(critical) > 0 {
		report.WriteString("\n## Critical Uncovered Paths\n\n")

		for packageName, paths := range critical {
			report.WriteString(fmt.Sprintf("### %s\n", packageName))
			for _, path := range paths {
				report.WriteString(fmt.Sprintf("- %s\n", path))
			}
			report.WriteString("\n")
		}
	}

	return report.String()
}
