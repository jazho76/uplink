package target

import (
	"errors"
	"regexp"
)

const (
	ConfigSource = "config"
	DefaultGroup = "ungrouped"
)

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type ID struct {
	Source string
	Key    string
}

func (id ID) String() string {
	if id.Source == ConfigSource {
		return id.Key
	}
	return id.Source + "/" + id.Key
}

func ValidateKey(key string) error {
	if key == "" {
		return errors.New("no name")
	}
	if !namePattern.MatchString(key) {
		return errors.New("name may only hold letters, digits, '.', '_' and '-'")
	}
	return nil
}

type Target struct {
	ID      ID
	Name    string
	Group   string
	Reach   Reach
	Check   string
	Power   *Power
	Logs    LogSource
	Watch   bool
	Metrics []Metric
	Modes   []Mode
}

func (t Target) DetectsMetrics() bool { return t.Metrics == nil }

func (t Target) Discovered() bool { return t.ID.Source != ConfigSource }

func (t Target) StartCommand() string {
	if t.Power == nil {
		return ""
	}
	return t.Power.Up
}

func (t Target) StopCommand() string {
	if t.Power == nil {
		return ""
	}
	return t.Power.Down
}

type Power struct {
	Up   string
	Down string
}
