package inventory

import (
	"path"
	"strings"

	"github.com/jazho76/uplink/internal/target"
)

type Overlay struct {
	Match  string
	Hide   *bool
	Adjust func(target.Target) target.Target
}

func (o Overlay) Matches(id target.ID) bool {
	matched, err := path.Match(o.Match, id.String())
	return err == nil && matched
}

func (o Overlay) Provider() string {
	provider, _, _ := strings.Cut(o.Match, "/")
	return provider
}
