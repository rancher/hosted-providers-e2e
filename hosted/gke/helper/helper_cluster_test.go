package helper

import (
	"reflect"
	"testing"
)

func TestParseGKEChannelVersions(t *testing.T) {
	serverConfig := []byte("Fetching server config...done.\n" + `{"channels":[{"channel":"RAPID","validVersions":["1.36.4-gke.2046000","1.35.8-gke.1796000"]},{"channel":"REGULAR","validVersions":["1.35.8-gke.1796000","1.34.12-gke.1011000"]}]}`)

	versions, err := parseGKEChannelVersions(serverConfig, "Rapid")
	if err != nil {
		t.Fatalf("parseGKEChannelVersions returned error: %v", err)
	}
	want := []string{"1.36.4-gke.2046000", "1.35.8-gke.1796000"}
	if !reflect.DeepEqual(versions, want) {
		t.Fatalf("got versions %v, want %v", versions, want)
	}
}

func TestFilterGKEUpgradeVersions(t *testing.T) {
	versions := []string{
		"1.38.1-gke.100",
		"1.37.2-gke.200",
		"1.36.5-gke.300",
		"1.36.4-gke.200",
		"1.35.9-gke.100",
	}

	filtered, err := filterGKEUpgradeVersions(versions, "1.35.8-gke.1796000")
	if err != nil {
		t.Fatalf("filterGKEUpgradeVersions returned error: %v", err)
	}
	want := []string{"1.36.5-gke.300", "1.36.4-gke.200"}
	if !reflect.DeepEqual(filtered, want) {
		t.Fatalf("got versions %v, want %v", filtered, want)
	}
}
