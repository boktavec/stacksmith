package core

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io"
	"sort"

	"gopkg.in/yaml.v3"
)

// snapshotFS holds a vendored, read-only copy of the foggy-blueprints catalog.
// go:embed bakes the listed files into the binary at build time, so the core
// stays offline and I/O-free. Paths are listed explicitly (no globs) so a stray
// file in the directory is never embedded by accident. Provenance and refresh
// steps are in the MANIFEST file.
//
//go:embed snapshot/foggy-blueprints/catalog.yaml snapshot/foggy-blueprints/MANIFEST
var snapshotFS embed.FS

const catalogPath = "snapshot/foggy-blueprints/catalog.yaml"

// catalogFile mirrors the upstream catalog.yaml. Every upstream field is
// declared so KnownFields(true) can reject anything unexpected; some fields
// are not read yet but are kept so a schema change in a refreshed snapshot
// fails loudly instead of being ignored.
type catalogFile struct {
	AWSProviderVersion string                  `yaml:"aws_provider_version"`
	Blueprints         map[string]catalogEntry `yaml:"blueprints"`
}

type catalogEntry struct {
	Summary            string                       `yaml:"summary"`
	Required           []string                     `yaml:"required"`
	Publishes          []string                     `yaml:"publishes"`
	Consumes           []string                     `yaml:"consumes"`
	AWSProviderVersion string                       `yaml:"aws_provider_version"`
	Providers          []catalogProvider            `yaml:"providers"`
	Tiers              map[string]map[string]string `yaml:"tiers"`
	Template           string                       `yaml:"template"`
	Module             string                       `yaml:"module"`
}

type catalogProvider struct {
	Name    string `yaml:"name"`
	Source  string `yaml:"source"`
	Version string `yaml:"version"`
}

// supportedBlueprint is what StackSmith can generate today; it must match the
// catalog's template and module paths for the same ID.
type supportedBlueprint struct{ template, module string }

var supportedBlueprints = map[string]supportedBlueprint{
	"network": {template: "templates/network.tf.tmpl", module: "modules/network"},
}

// CatalogEntry is one blueprint listed by the catalog.
type CatalogEntry struct {
	ID        string
	Summary   string
	Supported bool
}

// Catalog lists the blueprints in the embedded snapshot, sorted by ID. It
// decodes the embedded bytes on each call and keeps no global state.
func Catalog() ([]CatalogEntry, error) {
	data, err := snapshotFS.ReadFile(catalogPath)
	if err != nil {
		return nil, fmt.Errorf("read embedded catalog: %w", err)
	}
	file, err := decodeCatalog(data)
	if err != nil {
		return nil, fmt.Errorf("embedded catalog is invalid: %w", err)
	}
	entries := make([]CatalogEntry, 0, len(file.Blueprints))
	for id, blueprint := range file.Blueprints {
		_, supported := supportedBlueprints[id]
		entries = append(entries, CatalogEntry{ID: id, Summary: blueprint.Summary, Supported: supported})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries, nil
}

// decodeCatalog strictly decodes exactly one catalog document. yaml.v3 rejects
// duplicate mapping keys on its own; KnownFields(true) rejects unknown fields
// only where the target is a struct, so free-form maps such as tiers accept
// any key.
func decodeCatalog(data []byte) (catalogFile, error) {
	var file catalogFile
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&file); err != nil {
		return catalogFile{}, fmt.Errorf("decode catalog: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return catalogFile{}, errors.New("catalog must contain exactly one YAML document")
		}
		return catalogFile{}, fmt.Errorf("decode catalog: %w", err)
	}
	if len(file.Blueprints) == 0 {
		return catalogFile{}, errors.New("catalog has no blueprints")
	}
	return file, nil
}
