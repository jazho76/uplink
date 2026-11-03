package launcher

import (
	"slices"

	"github.com/jazho76/uplink/internal/target"
)

func (s State) Modes(t target.Target) []target.Mode {
	return target.AvailableModes(t, s.Observation(t.ID).Tools)
}

func (s State) ModeIndex() int {
	t, _ := s.selectedTarget()
	selected := slices.IndexFunc(s.Modes(t), func(mode target.Mode) bool { return mode.Name == s.modeName })
	return max(selected, 0)
}

func (s State) Mode() target.Mode {
	t, _ := s.selectedTarget()
	modes := s.Modes(t)
	if len(modes) == 0 {
		return target.Mode{}
	}
	return modes[s.ModeIndex()]
}

func (s *State) cycleMode(delta int) {
	t, _ := s.selectedTarget()
	modes := s.Modes(t)
	if len(modes) < 2 {
		return
	}
	s.modeName = modes[wrap(s.ModeIndex()+delta, len(modes))].Name
}

func wrap(i, n int) int {
	return ((i % n) + n) % n
}
