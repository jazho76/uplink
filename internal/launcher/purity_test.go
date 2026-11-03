package launcher

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func TestCoreRunsNothingAndReadsNothing(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".", "../inventory").Output()
	if err != nil {
		t.Fatal(err)
	}
	const internal = "github.com/jazho76/uplink/internal/"
	dependencies := strings.Fields(string(out))
	for _, forbidden := range []string{
		"os/exec",
		"gopkg.in/yaml.v3",
		internal + "catalog",
		internal + "config",
		internal + "inspection",
		internal + "process",
		internal + "system",
	} {
		if slices.Contains(dependencies, forbidden) {
			t.Errorf("the core must not depend on %s", forbidden)
		}
	}
}
