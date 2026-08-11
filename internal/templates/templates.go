package templates

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jazho76/uplink/internal/run"
)

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type Template struct {
	Name string
	Dir  string
}

func (t Template) File() string { return filepath.Join(t.Dir, "template.yaml") }

func (t Template) FetchExternals() string { return filepath.Join(t.Dir, "fetch-externals.sh") }

func (t Template) ApplyExternals() string {
	return filepath.Join(t.Dir, "provision", "apply-externals.sh")
}

func Root() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	// Still "vmm" after the rename: existing Lima instances bake this absolute
	// path into their TemplateDir param and mount it, so moving it would break
	// their next start.
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
		return Template{}, fmt.Errorf("no such template: %s", name)
	}
	return Template{Name: name, Dir: dir}, nil
}

func ValidName(name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("invalid name %q: use letters, digits, '.', '_', '-' and no leading dot", name)
	}
	return nil
}

func NameFromURL(url string) string {
	url = strings.TrimRight(url, "/")
	url = strings.TrimSuffix(url, ".git")
	if i := strings.LastIndexAny(url, "/:"); i >= 0 {
		url = url[i+1:]
	}
	return url
}

func Add(root, url, name string) (Template, error) {
	if err := ValidName(name); err != nil {
		return Template{}, err
	}
	dir := filepath.Join(root, name)
	if _, err := os.Stat(dir); err == nil {
		return Template{}, fmt.Errorf("template %q already exists", name)
	}
	if err := run.Stream("git", "clone", url, dir); err != nil {
		return Template{}, err
	}
	tmpl := Template{Name: name, Dir: dir}
	if _, err := os.Stat(tmpl.File()); err != nil {
		os.RemoveAll(dir)
		return Template{}, fmt.Errorf("%s is not a template: no template.yaml", url)
	}
	return tmpl, nil
}

func (t Template) Update() error {
	return run.Stream("git", "-C", t.Dir, "pull", "--ff-only")
}

func (t Template) Remove() error {
	return os.RemoveAll(t.Dir)
}

func (t Template) Origin() string {
	url, err := run.Output("git", "-C", t.Dir, "config", "--get", "remote.origin.url")
	if err != nil {
		return ""
	}
	return url
}

func (t Template) Dirty() bool {
	out, err := run.Output("git", "-C", t.Dir, "status", "--porcelain")
	return err == nil && out != ""
}
