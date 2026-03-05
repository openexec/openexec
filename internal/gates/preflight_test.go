package gates

import (
	"strings"
	"testing"
)

func TestContainsAny(t *testing.T) {
	tests := []struct {
		s          string
		substrings []string
		expected   bool
	}{
		{"hello world", []string{"hello", "earth"}, true},
		{"hello world", []string{"moon", "earth"}, false},
		{"hello world", []string{"world"}, true},
		{"", []string{"something"}, false},
	}

	for _, tt := range tests {
		if got := containsAny(tt.s, tt.substrings...); got != tt.expected {
			t.Errorf("containsAny(%q, %v) = %v, want %v", tt.s, tt.substrings, got, tt.expected)
		}
	}
}

func TestContainsAnyInSlice(t *testing.T) {
	tests := []struct {
		slice    []string
		values   []string
		expected bool
	}{
		{[]string{"a", "b", "c"}, []string{"b", "d"}, true},
		{[]string{"a", "b", "c"}, []string{"d", "e"}, false},
		{[]string{"a"}, []string{"a"}, true},
		{[]string{}, []string{"a"}, false},
	}

	for _, tt := range tests {
		if got := containsAnyInSlice(tt.slice, tt.values...); got != tt.expected {
			t.Errorf("containsAnyInSlice(%v, %v) = %v, want %v", tt.slice, tt.values, got, tt.expected)
		}
	}
}

func TestFormatPreflightReport(t *testing.T) {
	tests := []struct {
		name     string
		report   *PreflightReport
		contains string
	}{
		{
			name: "All passed",
			report: &PreflightReport{
				Passed:  true,
				Summary: "✓ All 3 preflight checks passed",
			},
			contains: "✓ All 3 preflight checks passed",
		},
		{
			name: "One failed",
			report: &PreflightReport{
				Passed: false,
				Checks: []PreflightCheck{
					{Name: "docker", Passed: false, Error: "not running", FixCommand: "start it"},
				},
			},
			contains: "## Preflight Checks FAILED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatPreflightReport(tt.report)
			if !strings.Contains(got, tt.contains) {
				t.Errorf("FormatPreflightReport() = %q, want it to contain %q", got, tt.contains)
			}
		})
	}
}
