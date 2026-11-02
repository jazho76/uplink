package selfupdate

import (
	"strings"
	"testing"
)

func TestAssetForReleasedPlatforms(t *testing.T) {
	for _, tc := range []struct{ goos, goarch, want string }{
		{"linux", "amd64", "uplink-linux-amd64"},
		{"linux", "arm64", "uplink-linux-arm64"},
		{"darwin", "amd64", "uplink-darwin-amd64"},
		{"darwin", "arm64", "uplink-darwin-arm64"},
	} {
		got, err := assetFor(tc.goos, tc.goarch)
		if err != nil {
			t.Errorf("%s/%s: %v", tc.goos, tc.goarch, err)
		}
		if got != tc.want {
			t.Errorf("%s/%s: want %q, got %q", tc.goos, tc.goarch, tc.want, got)
		}
	}
}

func TestAssetForRejectsUnreleasedPlatforms(t *testing.T) {
	for _, tc := range []struct{ goos, goarch string }{
		{"windows", "amd64"},
		{"linux", "riscv64"},
	} {
		_, err := assetFor(tc.goos, tc.goarch)
		if err == nil || !strings.Contains(err.Error(), tc.goos+"/"+tc.goarch) {
			t.Errorf("%s/%s: want the platform named, got %v", tc.goos, tc.goarch, err)
		}
	}
}
