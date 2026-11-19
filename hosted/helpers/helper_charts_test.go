package helpers

import "testing"

func TestStagedOperatorChartVersionConstraint(t *testing.T) {
	tests := []struct {
		name               string
		rancherFullVersion string
		wantConstraint     string
		wantError          bool
	}{
		{name: "head channel", rancherFullVersion: "head/2.16", wantConstraint: "~111"},
		{name: "prime devel channel", rancherFullVersion: "prime/devel/2.15", wantConstraint: "~110"},
		{name: "latest channel", rancherFullVersion: "latest/latest", wantConstraint: ""},
		{name: "missing version", rancherFullVersion: "head", wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := stagedOperatorChartVersionConstraint(test.rancherFullVersion)
			if (err != nil) != test.wantError {
				t.Fatalf("stagedOperatorChartVersionConstraint(%q) error = %v, wantError %t", test.rancherFullVersion, err, test.wantError)
			}
			if got != test.wantConstraint {
				t.Errorf("stagedOperatorChartVersionConstraint(%q) = %q, want %q", test.rancherFullVersion, got, test.wantConstraint)
			}
		})
	}
}
