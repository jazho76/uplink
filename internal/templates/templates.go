package templates

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Template struct {
	Name string
	Dir  string
}

func (p Template) File() string { return filepath.Join(p.Dir, "template.yaml") }

func (p Template) FetchExternals() string { return filepath.Join(p.Dir, "fetch-externals.sh") }

func (p Template) ApplyExternals() string {
	return filepath.Join(p.Dir, "provision", "apply-externals.sh")
}

func (p Template) Scalar(key string) string {
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
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(home, ".local", "share", "vmm", "templates")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	return root, nil
}

func All(root string) ([]Template, error) {
	matches, err := filepath.Glob(filepath.Join(root, "*", "template.yaml"))
	if err != nil {
		return nil, err
	}
	tmpls := make([]Template, 0, len(matches))
	for _, m := range matches {
		dir := filepath.Dir(m)
		tmpls = append(tmpls, Template{Name: filepath.Base(dir), Dir: dir})
	}
	sort.Slice(tmpls, func(i, j int) bool { return tmpls[i].Name < tmpls[j].Name })
	return tmpls, nil
}

func Find(root, name string) (Template, error) {
	dir := filepath.Join(root, name)
	if _, err := os.Stat(filepath.Join(dir, "template.yaml")); err != nil {
		return Template{}, fmt.Errorf("no such vm: %s", name)
	}
	return Template{Name: name, Dir: dir}, nil
}
