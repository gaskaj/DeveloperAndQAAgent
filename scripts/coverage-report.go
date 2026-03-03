// coverage-report generates detailed coverage reports and analysis.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/testing"
)

const (
	version = "1.0.0"
	
	// Exit codes
	exitSuccess         = 0
	exitError          = 1
	exitQualityGateFail = 2
)

// Config holds the configuration for coverage report generation.
type Config struct {
	ProfilePath    string
	OutputFormat   string
	OutputFile     string
	ConfigPath     string
	CheckGates     bool
	Verbose        bool
	ShowVersion    bool
}

func main() {
	config, err := parseFlags()
	if err != nil {
		log.Fatalf("Error parsing flags: %v", err)
	}

	if config.ShowVersion {
		fmt.Printf("coverage-report version %s\n", version)
		os.Exit(exitSuccess)
	}

	if config.ProfilePath == "" {
		log.Fatal("Coverage profile path is required")
	}

	// Create coverage analyzer
	analyzer := testing.NewCoverageAnalyzer()

	// Load coverage profile
	if config.Verbose {
		fmt.Printf("Loading coverage profile: %s\n", config.ProfilePath)
	}

	err = analyzer.LoadCoverageProfile(config.ProfilePath)
	if err != nil {
		log.Fatalf("Failed to load coverage profile: %v", err)
	}

	// Generate report based on format
	var report string
	switch config.OutputFormat {
	case "markdown", "md":
		report = analyzer.GenerateReport()
	case "json":
		report = generateJSONReport(analyzer)
	case "text":
		report = generateTextReport(analyzer)
	default:
		log.Fatalf("Unsupported output format: %s", config.OutputFormat)
	}

	// Output report
	if config.OutputFile != "" {
		err = writeReportToFile(report, config.OutputFile)
		if err != nil {
			log.Fatalf("Failed to write report to file: %v", err)
		}
		if config.Verbose {
			fmt.Printf("Report written to: %s\n", config.OutputFile)
		}
	} else {
		fmt.Print(report)
	}

	// Check quality gates if requested
	if config.CheckGates {
		if config.Verbose {
			fmt.Println("Checking quality gates...")
		}
		
		passed := checkQualityGates(analyzer, config.Verbose)
		if !passed {
			if config.Verbose {
				fmt.Println("Quality gates failed!")
			}
			os.Exit(exitQualityGateFail)
		} else {
			if config.Verbose {
				fmt.Println("All quality gates passed!")
			}
		}
	}

	os.Exit(exitSuccess)
}

// parseFlags parses command line flags and returns a Config.
func parseFlags() (*Config, error) {
	config := &Config{}

	flag.StringVar(&config.ProfilePath, "profile", "", "Path to coverage profile file")
	flag.StringVar(&config.OutputFormat, "format", "markdown", "Output format (markdown, json, text)")
	flag.StringVar(&config.OutputFile, "output", "", "Output file (default: stdout)")
	flag.StringVar(&config.ConfigPath, "config", "", "Path to quality gates configuration file")
	flag.BoolVar(&config.CheckGates, "check-gates", false, "Check quality gates")
	flag.BoolVar(&config.Verbose, "verbose", false, "Verbose output")
	flag.BoolVar(&config.ShowVersion, "version", false, "Show version and exit")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Generate detailed coverage reports from Go coverage profiles.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s -profile=coverage.out -format=markdown -output=report.md\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -profile=coverage.out -check-gates -verbose\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -profile=coverage.out -format=json | jq '.overall_coverage'\n", os.Args[0])
	}

	flag.Parse()

	return config, nil
}

// writeReportToFile writes the report to the specified file.
func writeReportToFile(report, filename string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(filename)
	if dir != "." {
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Write report to file
	err := os.WriteFile(filename, []byte(report), 0644)
	if err != nil {
		return fmt.Errorf("failed to write file %s: %w", filename, err)
	}

	return nil
}

// generateJSONReport generates a JSON format coverage report.
func generateJSONReport(analyzer *testing.CoverageAnalyzer) string {
	packageStats := analyzer.GetPackageStats()
	overallCoverage := analyzer.GetOverallCoverage()
	thresholds := analyzer.CheckThresholds()

	// Simple JSON generation (in a real implementation, use encoding/json)
	json := fmt.Sprintf(`{
  "overall_coverage": %.2f,
  "packages": {`, overallCoverage)

	first := true
	for packageName, stats := range packageStats {
		if !first {
			json += ","
		}
		first = false

		thresholdMet := "true"
		if !thresholds[packageName] {
			thresholdMet = "false"
		}

		json += fmt.Sprintf(`
    "%s": {
      "coverage": %.2f,
      "total_statements": %d,
      "covered_statements": %d,
      "threshold": %.1f,
      "threshold_met": %s
    }`, packageName, stats.CoveragePercent, stats.TotalStatements, 
			stats.CoveredStmts, analyzer.GetThreshold(packageName), thresholdMet)
	}

	json += `
  }
}`

	return json
}

// generateTextReport generates a plain text coverage report.
func generateTextReport(analyzer *testing.CoverageAnalyzer) string {
	report := fmt.Sprintf("Test Coverage Report\n")
	report += fmt.Sprintf("===================\n\n")
	report += fmt.Sprintf("Overall Coverage: %.2f%%\n\n", analyzer.GetOverallCoverage())

	report += fmt.Sprintf("Package Coverage:\n")
	report += fmt.Sprintf("-----------------\n")

	packageStats := analyzer.GetPackageStats()
	thresholds := analyzer.CheckThresholds()

	for packageName, stats := range packageStats {
		status := "PASS"
		if !thresholds[packageName] {
			status = "FAIL"
		}

		report += fmt.Sprintf("%-20s %6.2f%% [%6.1f%%] %s\n", 
			packageName, stats.CoveragePercent, 
			analyzer.GetThreshold(packageName), status)
	}

	// Critical uncovered paths
	critical := analyzer.FindCriticalUncoveredPaths()
	if len(critical) > 0 {
		report += fmt.Sprintf("\nCritical Uncovered Paths:\n")
		report += fmt.Sprintf("------------------------\n")

		for packageName, paths := range critical {
			report += fmt.Sprintf("\n%s:\n", packageName)
			for _, path := range paths {
				report += fmt.Sprintf("  - %s\n", path)
			}
		}
	}

	return report
}

// checkQualityGates checks if all quality gates pass.
func checkQualityGates(analyzer *testing.CoverageAnalyzer, verbose bool) bool {
	overallCoverage := analyzer.GetOverallCoverage()
	thresholds := analyzer.CheckThresholds()

	allPassed := true

	// Check overall coverage (minimum 80%)
	minOverallCoverage := 80.0
	if overallCoverage < minOverallCoverage {
		if verbose {
			fmt.Printf("FAIL: Overall coverage (%.2f%%) below minimum (%.1f%%)\n", 
				overallCoverage, minOverallCoverage)
		}
		allPassed = false
	} else if verbose {
		fmt.Printf("PASS: Overall coverage (%.2f%%) meets minimum (%.1f%%)\n", 
			overallCoverage, minOverallCoverage)
	}

	// Check package thresholds
	for packageName, passed := range thresholds {
		packageStats := analyzer.GetPackageStats()[packageName]
		threshold := analyzer.GetThreshold(packageName)

		if !passed {
			if verbose {
				fmt.Printf("FAIL: Package %s coverage (%.2f%%) below threshold (%.1f%%)\n",
					packageName, packageStats.CoveragePercent, threshold)
			}
			allPassed = false
		} else if verbose {
			fmt.Printf("PASS: Package %s coverage (%.2f%%) meets threshold (%.1f%%)\n",
				packageName, packageStats.CoveragePercent, threshold)
		}
	}

	return allPassed
}