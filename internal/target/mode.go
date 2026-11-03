package target

import "slices"

type Mode struct {
	Name        string
	Argv        []string
	Multiplexer string
	Requires    string
}

func AvailableModes(t Target, tools Fact[[]string]) []Mode {
	var modes []Mode
	for i, mode := range t.Modes {
		found := tools.Known && slices.Contains(tools.Value, mode.Requires)
		assumedDefault := !tools.Known && i == 0
		if mode.Requires == "" || found || assumedDefault {
			modes = append(modes, mode)
		}
	}
	return modes
}
