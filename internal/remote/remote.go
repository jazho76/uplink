package remote

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/jazho76/uplink/internal/probe"
	"github.com/jazho76/uplink/internal/target"
)

const Section = "remotes"

const listHeader = "remotes"

const defaultShell = "bash"

type Remote struct {
	Name     string            `yaml:"name"`
	SSH      string            `yaml:"ssh"`
	Identity string            `yaml:"identity"`
	Port     int               `yaml:"port"`
	SSHArgs  []string          `yaml:"sshArgs"`
	Shell    string            `yaml:"shell"`
	Init     string            `yaml:"init"`
	Modes    []target.ModeSpec `yaml:"modes"`
}

type Config []Remote

type Provider struct {
	remotes []entry

	mu     sync.Mutex
	status map[string]target.Status
}

type entry struct {
	remote   Remote
	identity string
	warning  string
	modes    []target.Mode
}

func New(cfg Config, configDir string) (*Provider, error) {
	p := &Provider{status: map[string]target.Status{}}

	seen := map[string]bool{}
	for i, r := range cfg {
		if r.Name == "" {
			return nil, fmt.Errorf("%s: entry %d has no name", Section, i+1)
		}
		if seen[r.Name] {
			return nil, fmt.Errorf("%s: duplicate remote %q", Section, r.Name)
		}
		seen[r.Name] = true

		if err := target.ValidateSpecs(Section+" "+r.Name, r.Modes); err != nil {
			return nil, err
		}

		identity, warning := resolveIdentity(r.Identity, configDir)
		e := entry{remote: r, identity: identity, warning: warning}
		e.modes = e.buildModes()
		p.remotes = append(p.remotes, e)
	}
	return p, nil
}

func (p *Provider) ID() string { return target.ProviderRemote }

func (p *Provider) List() ([]target.Target, error) {
	targets := make([]target.Target, 0, len(p.remotes))
	for _, e := range p.remotes {
		t := target.Target{
			Provider: p.ID(),
			Section:  listHeader,
			Name:     e.remote.Name,
			Status:   p.statusOf(e.remote.Name),
			Modes:    e.modes,
			Detail:   e.detail(),
		}
		targets = append(targets, t)
	}
	return targets, nil
}

func (p *Provider) Probe(name string) (probe.Stats, error) {
	e, ok := p.find(name)
	if !ok {
		return probe.Stats{}, fmt.Errorf("no such remote %q", name)
	}

	stats, err := probe.Run(e.probeArgv()...)

	status := target.StatusRunning
	if err != nil {
		status = target.StatusUnreachable
	}
	p.mu.Lock()
	p.status[name] = status
	p.mu.Unlock()

	return stats, err
}

func (p *Provider) statusOf(name string) target.Status {
	p.mu.Lock()
	defer p.mu.Unlock()
	if s, ok := p.status[name]; ok {
		return s
	}
	return target.StatusUnknown
}

func (p *Provider) find(name string) (entry, bool) {
	for _, e := range p.remotes {
		if e.remote.Name == name {
			return e, true
		}
	}
	return entry{}, false
}

func (e entry) detail() []target.Field {
	fields := []target.Field{{Key: "ssh", Value: e.remote.destination()}}
	if e.remote.Identity != "" {
		value := e.identity
		if e.warning != "" {
			value += "  (" + e.warning + ")"
		}
		fields = append(fields, target.Field{Key: "key", Value: value})
	}
	if e.remote.Port != 0 {
		fields = append(fields, target.Field{Key: "port", Value: strconv.Itoa(e.remote.Port)})
	}
	if e.remote.Init != "" {
		fields = append(fields, target.Field{Key: "init", Value: e.remote.Init})
	}
	return fields
}

func (r Remote) destination() string {
	if r.SSH != "" {
		return r.SSH
	}
	return r.Name
}

func (r Remote) shell() string {
	if r.Shell != "" {
		return r.Shell
	}
	return defaultShell
}

func (e entry) buildModes() []target.Mode {
	specs := e.remote.Modes
	if len(specs) == 0 {
		specs = []target.ModeSpec{{Name: "shell"}}
	}
	modes := make([]target.Mode, 0, len(specs))
	for _, s := range specs {
		modes = append(modes, target.Mode{Name: s.Name, Argv: e.launchArgv(s.Run), Back: s.Back})
	}
	return modes
}

func (e entry) launchArgv(payload string) []string {
	argv := append(e.sshFlags(), "-t", e.remote.destination())

	script := e.remote.script(payload)
	if script == "" {
		return argv
	}
	return append(argv, e.remote.shell(), "--login", "-c", quotedForRemoteShell(script))
}

func (e entry) probeArgv() []string {
	argv := append(e.sshFlags(), "-o", "BatchMode=yes", "-o", "ConnectTimeout=2")
	return append(argv, e.remote.destination(), "sh")
}

func (e entry) sshFlags() []string {
	argv := []string{"ssh"}
	if e.identity != "" {
		argv = append(argv, identityFlagsThatOutrankTheAgent(e.identity)...)
	}
	if e.remote.Port != 0 {
		argv = append(argv, "-p", strconv.Itoa(e.remote.Port))
	}
	return append(argv, e.remote.SSHArgs...)
}

func (r Remote) script(payload string) string {
	switch {
	case r.Init == "":
		return payload
	case payload == "":
		return r.Init + " && exec " + r.shell()
	default:
		return r.Init + " && " + payload
	}
}

func identityFlagsThatOutrankTheAgent(path string) []string {
	return []string{"-i", path, "-o", "IdentitiesOnly=yes"}
}

func quotedForRemoteShell(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func resolveIdentity(path, configDir string) (resolved, warning string) {
	if path == "" {
		return "", ""
	}

	switch {
	case strings.HasPrefix(path, "~/"):
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	default:
		path = os.ExpandEnv(path)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(configDir, path)
	}

	info, err := os.Stat(path)
	switch {
	case err != nil:
		return path, "missing"
	case info.IsDir():
		return path, "not a file"
	case info.Mode().Perm()&0o077 != 0:
		return path, "permissions too open, ssh will refuse it"
	}
	return path, ""
}
