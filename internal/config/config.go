package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type BucketConfig struct {
	Name    string `yaml:"name"`
	Region  string `yaml:"region,omitempty"`
	Profile string `yaml:"profile,omitempty"`
}

type Config struct {
	DefaultBucket string         `yaml:"default_bucket,omitempty"`
	Buckets       []BucketConfig `yaml:"buckets,omitempty"`
	Region        string         `yaml:"region,omitempty"`
	Profile       string         `yaml:"profile,omitempty"`

	path string `yaml:"-"`
}

func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "yelo", "config.yaml")
}

func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultPath()
	}

	cfg := &Config{path: path}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	cfg.path = path
	return cfg, nil
}

func (c *Config) Save() error {
	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	return os.WriteFile(c.path, data, 0o644)
}

func (c *Config) AddBucket(name, region, profile string) {
	for i, b := range c.Buckets {
		if b.Name == name {
			c.Buckets[i].Region = region
			c.Buckets[i].Profile = profile
			return
		}
	}
	c.Buckets = append(c.Buckets, BucketConfig{
		Name:    name,
		Region:  region,
		Profile: profile,
	})
}

func (c *Config) RemoveBucket(name string) bool {
	for i, b := range c.Buckets {
		if b.Name == name {
			c.Buckets = append(c.Buckets[:i], c.Buckets[i+1:]...)
			if c.DefaultBucket == name {
				c.DefaultBucket = ""
			}
			return true
		}
	}
	return false
}

func (c *Config) SetDefault(name string) error {
	for _, b := range c.Buckets {
		if b.Name == name {
			c.DefaultBucket = name
			return nil
		}
	}
	return fmt.Errorf("bucket %q not configured; add it first with: yelo buckets add %s", name, name)
}

func (c *Config) GetBucket(name string) *BucketConfig {
	for _, b := range c.Buckets {
		if b.Name == name {
			return &b
		}
	}
	return nil
}

func (c *Config) ResolveBucket(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	if c.DefaultBucket != "" {
		return c.DefaultBucket, nil
	}
	if len(c.Buckets) == 1 {
		return c.Buckets[0].Name, nil
	}
	return "", fmt.Errorf("no bucket specified; use --bucket or set a default with: yelo buckets default <name>")
}

func (c *Config) ResolveRegion(override, bucket string) string {
	if override != "" {
		return override
	}
	if b := c.GetBucket(bucket); b != nil && b.Region != "" {
		return b.Region
	}
	if c.Region != "" {
		return c.Region
	}
	return ""
}

func (c *Config) ResolveProfile(override, bucket string) string {
	if override != "" {
		return override
	}
	if b := c.GetBucket(bucket); b != nil && b.Profile != "" {
		return b.Profile
	}
	if c.Profile != "" {
		return c.Profile
	}
	return ""
}
