package helper

import (
	"testing"

	"github.com/Masterminds/semver/v3"
)

func TestSelectNextMinorVersion(t *testing.T) {
	current, err := semver.NewVersion("1.35.8-gke.1796000")
	if err != nil {
		t.Fatal(err)
	}

	got, err := selectNextMinorVersion(current, []string{
		"1.35.9-gke.100",
		"1.36.4-gke.1247000",
		"1.36.4-gke.2046000",
		"1.37.1-gke.100",
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "1.36.4-gke.2046000"; got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestSelectNextMinorVersionWithoutNextMinor(t *testing.T) {
	current, err := semver.NewVersion("1.35.8-gke.1796000")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := selectNextMinorVersion(current, []string{"1.35.9-gke.100", "1.37.1-gke.100"}); err == nil {
		t.Fatal("expected an error when no version exists for the next minor")
	}
}
