package lima

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/jazho76/vms/internal/run"
)

const bin = "limactl"

type Instance struct {
	Name         string
	Status       string
	CPUs         string
	Memory       string
	Disk         string
	SSHAddress   string
	SSHLocalPort string
	Dir          string
	VMType       string
	Arch         string
	Hostname     string
}

func (i Instance) Running() bool { return i.Status == "Running" }

const listFields = 11

const listFormat = "{{.Name}}\t{{.Status}}\t{{.CPUs}}\t{{.Memory}}\t{{.Disk}}\t" +
	"{{.SSHAddress}}\t{{.SSHLocalPort}}\t{{.Dir}}\t{{.VMType}}\t{{.Arch}}\t{{.Hostname}}"

func List() ([]Instance, error) {
	out, err := run.Output(bin, "list", "--format", listFormat)
	if err != nil {
		return nil, err
	}
	var instances []Instance
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Split(line, "\t")
		for len(f) < listFields {
			f = append(f, "")
		}
		instances = append(instances, Instance{
			Name: f[0], Status: f[1], CPUs: f[2], Memory: f[3], Disk: f[4],
			SSHAddress: f[5], SSHLocalPort: f[6], Dir: f[7],
			VMType: f[8], Arch: f[9], Hostname: f[10],
		})
	}
	return instances, nil
}

func Get(name string) (Instance, bool) {
	instances, err := List()
	if err != nil {
		return Instance{}, false
	}
	for _, i := range instances {
		if i.Name == name {
			return i, true
		}
	}
	return Instance{}, false
}

func Start(name string) error  { return run.Stream(bin, "start", name) }
func Stop(name string) error   { return run.Silent(bin, "stop", name) }
func Delete(name string) error { return run.Silent(bin, "delete", name) }

func StartSilent(name string) error { return run.Silent(bin, "start", name) }

func Create(name, template string, params ...string) error {
	args := []string{"start", "--tty=false", "--name=" + name}
	for _, p := range params {
		args = append(args, "--param", p)
	}
	args = append(args, template)
	if err := run.Stream(bin, args...); err != nil {
		return err
	}
	return Stop(name)
}

func ShellArgv(name string) []string {
	return []string{bin, "shell", name, "tmux", "new-session", "-A", "-s", "0"}
}

func Copy(src, name, dst string) error {
	return run.Silent(bin, "copy", src, name+":"+dst)
}

func SetAutostart(name string, enabled bool) error {
	flag := "--enabled=false"
	if enabled {
		flag = "--enabled"
	}
	return run.Silent(bin, "start-at-login", name, flag, "-y")
}

func AutostartEnabled(name string) bool {
	_, err := os.Stat(filepath.Join(unitDir(), "lima-vm@"+name+".service"))
	return err == nil
}

func unitDir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "systemd", "user")
}
