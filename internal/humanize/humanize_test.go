package humanize

import (
	"testing"
	"time"
)

func TestBytes(t *testing.T) {
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
		if got := Bytes(c.in); got != c.want {
			t.Errorf("Bytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDuration(t *testing.T) {
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
		if got := Duration(c.in); got != c.want {
			t.Errorf("Duration(%s) = %q, want %q", c.in, got, c.want)
		}
	}
}
