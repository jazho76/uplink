package target

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

	"gopkg.in/yaml.v3"
)

const anchorPrefix = "x-"

type Entry struct {
	Provider string `yaml:"provider"`
	Profile  string `yaml:"profile"`
}

type Document struct {
	Profiles map[string]Spec `yaml:"profiles"`
	Targets  []Spec          `yaml:"targets"`
	Discover []Entry         `yaml:"discover"`
	Extra    map[string]any  `yaml:",inline"`
}

func (d Document) Config() Source { return configSource(d.Targets) }

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
			errs = append(errs, fmt.Errorf("%s: unknown section %q; use profiles, targets, discover, or an %s key for anchors", path, key, anchorPrefix))
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
