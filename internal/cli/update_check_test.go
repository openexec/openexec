package cli

import "testing"

func TestIsNewer(t *testing.T) {
	tests := []struct {
		latest  string
		current string
		want    bool
	}{
		{"0.8.1", "0.8.0", true},
		{"0.9.0", "0.8.0", true},
		{"1.0.0", "0.8.0", true},
		{"0.8.0", "0.8.0", false},
		{"0.7.0", "0.8.0", false},
		{"0.8.0", "0.9.0", false},
		{"0.8.0", "1.0.0", false},
		{"0.8.1", "0.8.1", false},
		{"0.10.0", "0.9.0", true},
		{"1.0.0", "0.99.0", true},
	}

	for _, tc := range tests {
		t.Run(tc.latest+"_vs_"+tc.current, func(t *testing.T) {
			got := isNewer(tc.latest, tc.current)
			if got != tc.want {
				t.Errorf("isNewer(%q, %q) = %v, want %v", tc.latest, tc.current, got, tc.want)
			}
		})
	}
}
