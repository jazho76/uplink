package presets

import "github.com/jazho76/uplink/internal/stats"

var Linux = []stats.Metric{
	{Type: stats.TypeText, Label: "up", Unit: stats.UnitSeconds, Value: "cut -d ' ' -f 1 /proc/uptime"},
	{Type: stats.TypeGauge, Label: "load", Value: "cut -d ' ' -f 1 /proc/loadavg", Max: "nproc"},
	{
		Type:  stats.TypeGauge,
		Label: "memory",
		Unit:  stats.UnitBytes,
		Value: `awk '/^MemTotal:/{t=$2} /^MemAvailable:/{a=$2} END{print (t-a)*1024}' /proc/meminfo`,
		Max:   `awk '/^MemTotal:/{print $2*1024}' /proc/meminfo`,
	},
	{
		Type:  stats.TypeGauge,
		Label: "disk",
		Unit:  stats.UnitBytes,
		Value: `df -P -k / | awk 'NR==2{print $3*1024}'`,
		Max:   `df -P -k / | awk 'NR==2{print $2*1024}'`,
	},
}

var MacOS = []stats.Metric{
	{
		Type:  stats.TypeText,
		Label: "up",
		Unit:  stats.UnitSeconds,
		Value: `echo $(( $(date +%s) - $(sysctl -n kern.boottime | awk '{print $4}' | tr -d ,) ))`,
	},
	{Type: stats.TypeGauge, Label: "load", Value: "sysctl -n vm.loadavg | awk '{print $2}'", Max: "sysctl -n hw.ncpu"},
	{
		Type:  stats.TypeGauge,
		Label: "memory",
		Unit:  stats.UnitBytes,
		Value: `vm_stat | awk '/page size of/{p=$8} /Pages active/{a=$3} /Pages wired down/{w=$4} /occupied by compressor/{c=$5} END{print (a+w+c)*p}'`,
		Max:   "sysctl -n hw.memsize",
	},
	{
		Type:  stats.TypeGauge,
		Label: "disk",
		Unit:  stats.UnitBytes,
		Value: `df -P -k /System/Volumes/Data | awk 'NR==2{print $3*1024}'`,
		Max:   `df -P -k /System/Volumes/Data | awk 'NR==2{print $2*1024}'`,
	},
}

func For(goos string) []stats.Metric {
	switch goos {
	case "linux":
		return Linux
	case "darwin":
		return MacOS
	default:
		return nil
	}
}
