package rules

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LoadFromDir reads all YAML rule files from a directory.
//
// Each file can contain one rule. Files must have .yaml or .yml extension.
// Rules with enabled=false are loaded but skipped during evaluation.
//
// Returns an error if any file is malformed — we fail loudly rather than
// silently ignoring bad rules.
func LoadFromDir(dir string) ([]*Rule, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading rules directory %s: %w", dir, err)
	}

	var rules []*Rule
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		rule, err := LoadFromFile(path)
		if err != nil {
			return nil, fmt.Errorf("loading rule from %s: %w", path, err)
		}

		rules = append(rules, rule)
	}

	return rules, nil
}

// LoadFromFile reads a single rule from a YAML file.
func LoadFromFile(path string) (*Rule, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path comes from trusted rules directory, not user input
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	return Parse(data)
}

// Parse deserializes a YAML byte slice into a Rule.
//
// Validates required fields after parsing — a rule must have
// at minimum an id, name, and type.
func Parse(data []byte) (*Rule, error) {
	var rule Rule
	if err := yaml.Unmarshal(data, &rule); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}

	if err := validate(&rule); err != nil {
		return nil, err
	}

	return &rule, nil
}

// validate checks that a rule has all required fields.
func validate(r *Rule) error {
	if r.ID == "" {
		return fmt.Errorf("rule missing required field: id")
	}
	if r.Name == "" {
		return fmt.Errorf("rule %s: missing required field: name", r.ID)
	}

	switch r.Type {
	case RuleTypeSingle:
		if len(r.Conditions) == 0 {
			return fmt.Errorf("rule %s: single rule must have at least one condition", r.ID)
		}
	case RuleTypeThreshold:
		if len(r.Conditions) == 0 {
			return fmt.Errorf("rule %s: threshold rule must have conditions", r.ID)
		}
		if r.Threshold <= 0 {
			return fmt.Errorf("rule %s: threshold must be > 0", r.ID)
		}
		if r.Window <= 0 {
			return fmt.Errorf("rule %s: window must be > 0", r.ID)
		}
		if len(r.GroupBy) == 0 {
			return fmt.Errorf("rule %s: threshold rule must have group_by", r.ID)
		}
	case RuleTypeSequence:
		if len(r.Steps) < 2 {
			return fmt.Errorf("rule %s: sequence rule must have at least 2 steps", r.ID)
		}
		if r.Window <= 0 {
			return fmt.Errorf("rule %s: window must be > 0", r.ID)
		}
		if len(r.GroupBy) == 0 {
			return fmt.Errorf("rule %s: sequence rule must have group_by", r.ID)
		}
	default:
		return fmt.Errorf("rule %s: unknown type %q (must be single, threshold, or sequence)", r.ID, r.Type)
	}

	return nil
}
