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

	"github.com/jazho76/vmm/internal/ui"
	"github.com/jazho76/vmm/internal/version"
)

const (
	repo  = "jazho76/vmm"
	asset = "vmm-linux-amd64"
)

var client = &http.Client{Timeout: 60 * time.Second}

func Run() error {
	if version.Version == "dev" {
		return fmt.Errorf("not a release build; reinstall with the install.sh bootstrap instead")
	}
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return fmt.Errorf("self-update supports linux/amd64 only (got %s/%s); build from source", runtime.GOOS, runtime.GOARCH)
	}

	latest, err := latestTag()
	if err != nil {
		return err
	}
	if latest == version.Version {
		ui.Info("Already on the latest version (%s)", version.Version)
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}

	ui.Info("Updating vmm %s -> %s", version.Version, latest)
	if err := replace(exe); err != nil {
		return err
	}
	ui.Info("Updated vmm to %s", latest)
	return nil
}

func latestTag() (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "vmm")

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

func replace(exe string) error {
	url := fmt.Sprintf("https://github.com/%s/releases/latest/download/%s", repo, asset)
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("could not download %s: %s", asset, resp.Status)
	}

	tmp, err := os.CreateTemp(filepath.Dir(exe), ".vmm-*.tmp")
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
