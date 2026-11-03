package target

import "slices"

var localPrefix = []string{"sh", "-c"}

type Reach struct {
	Interactive []string
	Background  []string
}

func (r Reach) Session(command string) []string {
	return withCommand(r.Interactive, command)
}

func (r Reach) Probe(command string) []string {
	if len(r.Background) > 0 {
		return withCommand(r.Background, command)
	}
	return r.Session(command)
}

func withCommand(prefix []string, command string) []string {
	if len(prefix) == 0 {
		prefix = localPrefix
	}
	return append(slices.Clone(prefix), command)
}
