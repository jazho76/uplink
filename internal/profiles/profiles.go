package profiles

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Profile struct {
	Name string
	Dir  string
}

func (p Profile) File() string { return filepath.Join(p.Dir, "profile.yaml") }

func (p Profile) FetchExternals() string { return filepath.Join(p.Dir, "fetch-externals.sh") }

func (p Profile) ApplyExternals() string {
	return filepath.Join(p.Dir, "provision", "apply-externals.sh")
}

func (p Profile) Scalar(key string) string {
	f, err := os.Open(p.File())
	if err != nil {
		return ""
	}
	defer f.Close()

	prefix := key + ":"
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		val := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		if i := strings.IndexByte(val, '#'); i >= 0 {
			val = strings.TrimSpace(val[:i])
		}
		return strings.Trim(val, `"`)
	}
	return ""
}

func Root() (string, error) {
	var candidates []string
	if exe, err := os.Executable(); err == nil {
		exe, _ = filepath.EvalSymlinks(exe)
		dir := filepath.Dir(exe)
		candidates = append(candidates, dir, filepath.Dir(dir))
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd, filepath.Dir(cwd))
	}

	for _, dir := range candidates {
		if matches, _ := filepath.Glob(filepath.Join(dir, "*", "profile.yaml")); len(matches) > 0 {
			return dir, nil
		}
	}
	return "", fmt.Errorf("no VM profiles found near %s", strings.Join(candidates, ", "))
}

func All(root string) ([]Profile, error) {
	matches, err := filepath.Glob(filepath.Join(root, "*", "profile.yaml"))
	if err != nil {
		return nil, err
	}
	profiles := make([]Profile, 0, len(matches))
	for _, m := range matches {
		dir := filepath.Dir(m)
		profiles = append(profiles, Profile{Name: filepath.Base(dir), Dir: dir})
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
	return profiles, nil
}

func Find(root, name string) (Profile, error) {
	dir := filepath.Join(root, name)
	if _, err := os.Stat(filepath.Join(dir, "profile.yaml")); err != nil {
		return Profile{}, fmt.Errorf("no such vm: %s", name)
	}
	return Profile{Name: name, Dir: dir}, nil
}
