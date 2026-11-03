package stats

import (
	"slices"

	"github.com/jazho76/uplink/internal/target"
)

const (
	PresetLinux     = "linux"
	PresetMacOS     = "macos"
	PresetContainer = "container"
)

var linux = []target.Metric{
	{Type: target.TypeText, Label: "up", Unit: target.UnitSeconds, Value: "cut -d ' ' -f 1 /proc/uptime"},
	{Type: target.TypeGauge, Label: "load", Value: "cut -d ' ' -f 1 /proc/loadavg", Max: "nproc"},
	{
		Type:  target.TypeGauge,
		Label: "memory",
		Unit:  target.UnitBytes,
		Value: `awk '/^MemTotal:/{t=$2} /^MemAvailable:/{a=$2} END{print (t-a)*1024}' /proc/meminfo`,
		Max:   `awk '/^MemTotal:/{print $2*1024}' /proc/meminfo`,
	},
	{
		Type:  target.TypeGauge,
		Label: "disk",
		Unit:  target.UnitBytes,
		Value: `df -P -k / | awk 'NR==2{print $3*1024}'`,
		Max:   `df -P -k / | awk 'NR==2{print $2*1024}'`,
	},
}

var macos = []target.Metric{
	{
		Type:  target.TypeText,
		Label: "up",
		Unit:  target.UnitSeconds,
		Value: `echo $(( $(date +%s) - $(sysctl -n kern.boottime | awk '{print $4}' | tr -d ,) ))`,
	},
	{Type: target.TypeGauge, Label: "load", Value: "sysctl -n vm.loadavg | awk '{print $2}'", Max: "sysctl -n hw.ncpu"},
	{
		Type:  target.TypeGauge,
		Label: "memory",
		Unit:  target.UnitBytes,
		Value: `vm_stat | awk '/page size of/{p=$8} /Pages active/{a=$3} /Pages wired down/{w=$4} /occupied by compressor/{c=$5} END{print (a+w+c)*p}'`,
		Max:   "sysctl -n hw.memsize",
	},
	{
		Type:  target.TypeGauge,
		Label: "disk",
		Unit:  target.UnitBytes,
		Value: `df -P -k /System/Volumes/Data | awk 'NR==2{print $3*1024}'`,
		Max:   `df -P -k /System/Volumes/Data | awk 'NR==2{print $2*1024}'`,
	},
}

var container = []target.Metric{{
	Type:  target.TypeGauge,
	Label: "memory",
	Unit:  target.UnitBytes,
	Value: "cat /sys/fs/cgroup/memory.current",
	Max: `limit=$(cat /sys/fs/cgroup/memory.max)
[ "$limit" = max ] && limit=$(awk '/^MemTotal:/{print $2 * 1024}' /proc/meminfo)
echo "$limit"`,
}}

var presets = map[string][]target.Metric{
	PresetLinux:     linux,
	PresetMacOS:     macos,
	PresetContainer: container,
}

func Preset(name string) ([]target.Metric, bool) {
	metrics, ok := presets[name]
	return slices.Clone(metrics), ok
}

func ForOS(goos string) []target.Metric {
	switch goos {
	case "linux":
		return slices.Clone(linux)
	case "darwin":
		return slices.Clone(macos)
	default:
		return nil
	}
}
