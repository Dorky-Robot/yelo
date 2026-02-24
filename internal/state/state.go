package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type State struct {
	Bucket string `json:"bucket,omitempty"`
	Prefix string `json:"prefix,omitempty"`

	path string
}

func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "yelo", "state.json")
}

func Load(path string) (*State, error) {
	if path == "" {
		path = DefaultPath()
	}

	s := &State{path: path}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading state: %w", err)
	}

	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("parsing state: %w", err)
	}
	s.path = path
	return s, nil
}

func (s *State) Save() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating state dir: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	return os.WriteFile(s.path, data, 0o644)
}

func (s *State) SetBucket(bucket string) {
	s.Bucket = bucket
	s.Prefix = ""
}
