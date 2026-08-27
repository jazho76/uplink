package lima

import (
	"slices"
	"testing"
)

func TestSectionSaysWhyThereAreNoVMs(t *testing.T) {
	installed := (&Provider{hasBin: true}).Section()
	if installed.Name != listHeader {
		t.Fatalf("section header = %q, want %q", installed.Name, listHeader)
	}
	if !slices.Contains(installed.Placeholder, createHint) {
		t.Errorf("an installed lima with no instances should point at create, got %v", installed.Placeholder)
	}

	missing := (&Provider{}).Section()
	if missing.Name != listHeader {
		t.Errorf("the section keeps its header either way, got %q", missing.Name)
	}
	if len(missing.Placeholder) == 0 {
		t.Fatal("a missing limactl should say so")
	}
	if slices.Contains(missing.Placeholder, createHint) {
		t.Errorf("pointing at create is useless without limactl, got %v", missing.Placeholder)
	}
}
