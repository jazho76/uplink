package target

import (
	"slices"
	"strings"
)

const (
	PresetLinux     = "linux"
	PresetMacOS     = "macos"
	PresetContainer = "container"
)

var linuxMetrics = []Metric{
	{Type: TypeText, Label: "up", Unit: UnitSeconds, Value: "cut -d ' ' -f 1 /proc/uptime"},
	{Type: TypeGauge, Label: "load", Value: "cut -d ' ' -f 1 /proc/loadavg", Max: "nproc"},
	{
		Type:  TypeGauge,
		Label: "memory",
		Unit:  UnitBytes,
		Value: `awk '/^MemTotal:/{t=$2} /^MemAvailable:/{a=$2} END{print (t-a)*1024}' /proc/meminfo`,
		Max:   `awk '/^MemTotal:/{print $2*1024}' /proc/meminfo`,
	},
	{
		Type:  TypeGauge,
		Label: "disk",
		Unit:  UnitBytes,
		Value: `df -P -k / | awk 'NR==2{print $3*1024}'`,
		Max:   `df -P -k / | awk 'NR==2{print $2*1024}'`,
	},
}

var macosMetrics = []Metric{
	{
		Type:  TypeText,
		Label: "up",
		Unit:  UnitSeconds,
		Value: `echo $(( $(date +%s) - $(sysctl -n kern.boottime | awk '{print $4}' | tr -d ,) ))`,
	},
	{Type: TypeGauge, Label: "load", Value: "sysctl -n vm.loadavg | awk '{print $2}'", Max: "sysctl -n hw.ncpu"},
	{
		Type:  TypeGauge,
		Label: "memory",
		Unit:  UnitBytes,
		Value: `vm_stat | awk '/page size of/{p=$8} /Pages active/{a=$3} /Pages wired down/{w=$4} /occupied by compressor/{c=$5} END{print (a+w+c)*p}'`,
		Max:   "sysctl -n hw.memsize",
	},
	{
		Type:  TypeGauge,
		Label: "disk",
		Unit:  UnitBytes,
		Value: `df -P -k /System/Volumes/Data | awk 'NR==2{print $3*1024}'`,
		Max:   `df -P -k /System/Volumes/Data | awk 'NR==2{print $2*1024}'`,
	},
}

var containerMetrics = []Metric{{
	Type:  TypeGauge,
	Label: "memory",
	Unit:  UnitBytes,
	Value: "cat /sys/fs/cgroup/memory.current",
	Max: `limit=$(cat /sys/fs/cgroup/memory.max)
[ "$limit" = max ] && limit=$(awk '/^MemTotal:/{print $2 * 1024}' /proc/meminfo)
echo "$limit"`,
}}

var presets = map[string][]Metric{
	PresetLinux:     linuxMetrics,
	PresetMacOS:     macosMetrics,
	PresetContainer: containerMetrics,
}

func Preset(name string) ([]Metric, bool) {
	metrics, ok := presets[name]
	return slices.Clone(metrics), ok
}

func DetectedMetrics() []Metric { return nil }

func BuiltinMetrics(os string) []Metric {
	switch strings.ToLower(os) {
	case "linux":
		return slices.Clone(linuxMetrics)
	case "darwin":
		return slices.Clone(macosMetrics)
	default:
		return []Metric{}
	}
}
