package tui

import (
	"testing"
	"time"
)

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		in   uint64
		want string
	}{
		{0, "0B"},
		{512, "512B"},
		{1024, "1KiB"},
		{1536, "1.5KiB"},
		{12 << 30, "12GiB"},
		{12522029056, "11.7GiB"},
		{1 << 50, "1PiB"},
	}
	for _, c := range cases {
		if got := formatBytes(c.in); got != c.want {
			t.Errorf("formatBytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, ""},
		{-time.Second, ""},
		{45 * time.Second, "45s"},
		{90 * time.Second, "1m"},
		{time.Hour + 30*time.Minute, "1h 30m"},
		{25 * time.Hour, "1d 1h"},
		{73 * time.Hour, "3d 1h"},
	}
	for _, c := range cases {
		if got := formatDuration(c.in); got != c.want {
			t.Errorf("formatDuration(%s) = %q, want %q", c.in, got, c.want)
		}
	}
}
