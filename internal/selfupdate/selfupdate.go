package selfupdate

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/jazho76/uplink/internal/version"
)

const repo = "jazho76/uplink"

var client = &http.Client{Timeout: 60 * time.Second}

func assetFor(goos, goarch string) (string, error) {
	switch goos + "/" + goarch {
	case "linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64":
		return "uplink-" + goos + "-" + goarch, nil
	}
	return "", fmt.Errorf("no release for %s/%s; build from source", goos, goarch)
}

func Run() error {
	if version.Version == "dev" {
		return fmt.Errorf("not a release build; reinstall with the install.sh bootstrap instead")
	}
	asset, err := assetFor(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}

	latest, err := latestTag()
	if err != nil {
		return err
	}
	if latest == version.Version {
		fmt.Printf("Already on the latest version (%s)\n", version.Version)
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}

	fmt.Printf("Updating uplink %s -> %s\n", version.Version, latest)
	if err := replace(exe, asset); err != nil {
		return err
	}
	fmt.Printf("Updated uplink to %s\n", latest)
	return nil
}

func latestTag() (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "uplink")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("could not query latest release: %s", resp.Status)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	if release.TagName == "" {
		return "", fmt.Errorf("latest release has no tag")
	}
	return release.TagName, nil
}

func replace(exe, asset string) error {
	url := fmt.Sprintf("https://github.com/%s/releases/latest/download/%s", repo, asset)
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("could not download %s: %s", asset, resp.Status)
	}

	tmp, err := os.CreateTemp(filepath.Dir(exe), ".uplink-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return err
	}
	return os.Rename(tmpPath, exe)
}
