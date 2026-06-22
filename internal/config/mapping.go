package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// MappingEntry is a single entry in the local film-to-TMDB mapping.
type MappingEntry struct {
	TMDBID int    `yaml:"tmdb_id"`
	Type   string `yaml:"type"` // "movie" or "tv"
	Title  string `yaml:"title,omitempty"`
}

// MappingTable maps a normalized title to its TMDB info.
type MappingTable map[string]MappingEntry

// LoadMapping reads the YAML mapping file and returns a lookup table.
func LoadMapping(path string) (MappingTable, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // mapping is optional
		}
		return nil, fmt.Errorf("read mapping file: %w", err)
	}

	var raw map[string]MappingEntry
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse mapping file: %w", err)
	}
	return MappingTable(raw), nil
}

// Lookup searches the mapping table for a title. Returns the entry and true if found.
func (m MappingTable) Lookup(title string) (MappingEntry, bool) {
	if m == nil {
		return MappingEntry{}, false
	}
	e, ok := m[title]
	return e, ok
}
