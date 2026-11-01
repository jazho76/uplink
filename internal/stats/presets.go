package stats

import (
	"fmt"
	"slices"
)

const (
	PresetLinux     = "linux"
	PresetMacOS     = "macos"
	PresetContainer = "container"
)

var linux = []Metric{
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

var macos = []Metric{
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

var container = []Metric{{
	Type:  TypeGauge,
	Label: "memory",
	Unit:  UnitBytes,
	Value: "cat /sys/fs/cgroup/memory.current",
	Max: `limit=$(cat /sys/fs/cgroup/memory.max)
[ "$limit" = max ] && limit=$(awk '/^MemTotal:/{print $2 * 1024}' /proc/meminfo)
echo "$limit"`,
}}

var presets = map[string][]Metric{
	PresetLinux:     linux,
	PresetMacOS:     macos,
	PresetContainer: container,
}

func Preset(name string) ([]Metric, bool) {
	metrics, ok := presets[name]
	return slices.Clone(metrics), ok
}

func ForOS(goos string) []Metric {
	switch goos {
	case "linux":
		return slices.Clone(linux)
	case "darwin":
		return slices.Clone(macos)
	default:
		return nil
	}
}

func Expand(metrics []Metric) ([]Metric, error) {
	if metrics == nil {
		return nil, nil
	}
	expanded := []Metric{}
	for _, m := range metrics {
		if m.Preset == "" {
			expanded = append(expanded, m)
			continue
		}
		preset, ok := presets[m.Preset]
		if !ok {
			return nil, fmt.Errorf("unknown preset %q", m.Preset)
		}
		expanded = append(expanded, preset...)
	}
	return expanded, nil
}
