package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/jazho76/uplink/internal/target"
	"gopkg.in/yaml.v3"
)

const anchorPrefix = "x-"

type Document struct {
	Targets     []Spec         `yaml:"targets"`
	Discover    []string       `yaml:"discover"`
	DefaultMode string         `yaml:"default_mode"`
	Discovered  []overlaySpec  `yaml:"discovered"`
	Extra       map[string]any `yaml:",inline"`
}

func Read(path string) (Document, error) {
	var doc Document
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return doc, nil
	case err != nil:
		return doc, err
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return Document{}, decodeError(path, err)
	}

	var errs []error
	for _, key := range slices.Sorted(maps.Keys(doc.Extra)) {
		if !strings.HasPrefix(key, anchorPrefix) {
			errs = append(errs, fmt.Errorf("%s: unknown section %q; use targets, discover, discovered, default_mode, or an %s key for anchors", path, key, anchorPrefix))
		}
	}
	if len(errs) > 0 {
		return Document{}, errors.Join(errs...)
	}
	return doc, nil
}

func decodeError(path string, err error) error {
	var typeErr *yaml.TypeError
	if !errors.As(err, &typeErr) {
		return fmt.Errorf("%s: %w", path, err)
	}
	errs := make([]error, len(typeErr.Errors))
	for i, message := range typeErr.Errors {
		errs[i] = fmt.Errorf("%s: %s", path, message)
	}
	return errors.Join(errs...)
}

func (d Document) Resolve() ([]target.Target, error) {
	var (
		targets []target.Target
		errs    []error
	)
	for i, spec := range d.Targets {
		t, err := spec.resolve()
		if err != nil {
			errs = append(errs, fmt.Errorf("target %s: %w", label(spec, i), err))
			continue
		}
		targets = append(targets, t)
	}
	return targets, errors.Join(errs...)
}

func label(spec Spec, index int) string {
	if spec.Name == "" {
		return fmt.Sprintf("#%d", index+1)
	}
	return fmt.Sprintf("%q", spec.Name)
}
