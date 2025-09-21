package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/furisto/construct/shared"
	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
)

type Store struct {
	settings map[string]any
	fs       *afero.Afero
	userInfo shared.UserInfo
}

func NewStore(fs *afero.Afero, userInfo shared.UserInfo) (*Store, error) {
	store := &Store{
		settings: make(map[string]any),
		fs:       fs,
		userInfo: userInfo,
	}
	err := store.load()
	if err != nil {
		return nil, err
	}
	return store, nil
}

func (c *Store) load() error {
	constructDir, err := c.userInfo.ConstructConfigDir()
	if err != nil {
		return fmt.Errorf("failed to retrieve construct config directory: %w", err)
	}

	settingsFile := filepath.Join(constructDir, "config.yaml")

	exists, err := c.fs.Exists(settingsFile)
	if err != nil {
		return fmt.Errorf("failed to check config file: %w", err)
	}

	if !exists {
		return nil
	}

	content, err := c.fs.ReadFile(settingsFile)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var new map[string]any
	if err := yaml.Unmarshal(content, &new); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}
	c.settings = new

	return nil
}

func (c *Store) Get(key string) (Value, bool) {
	raw, found := getNestedValue(c.settings, key)
	if !found {
		return Value{}, false
	}

	return Value{raw: raw}, true
}

func (c *Store) GetOrDefault(key string, defaultValue Value) (Value, bool) {
	raw, found := getNestedValue(c.settings, key)
	if !found {
		return defaultValue, false
	}
	return Value{raw: raw}, true
}

func (c *Store) Set(key string, value any) error {
	return setNestedValue(c.settings, key, value)
}

func (c *Store) Flush(fs *afero.Afero) error {
	output, err := MarshalYAMLWithSpacing(c.settings)
	if err != nil {
		return err
	}

	configDir, err := c.userInfo.ConstructConfigDir()
	if err != nil {
		return err
	}

	configPath := filepath.Join(configDir, "config.yaml")
	return c.fs.WriteFile(configPath, output, 0600)
}

func (c *Store) Delete(key string) error {
	err := unsetNestedValue(c.settings, key)
	if err != nil {
		return err
	}
	return nil
}

func MarshalYAMLWithSpacing(v any) ([]byte, error) {
	data, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	var result []string

	for i, line := range lines {
		if i > 0 && len(line) > 0 && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			result = append(result, "")
		}
		result = append(result, line)
	}

	return []byte(strings.Join(result, "\n")), nil
}

type Value struct {
	raw any
}

func (v Value) String() (string, bool) {
	if str, ok := v.raw.(string); ok {
		return str, true
	}
	return "", false
}

func (v Value) Int() (int64, bool) {
	if i, ok := v.raw.(int64); ok {
		return i, true
	}
	if i, ok := v.raw.(int); ok {
		return int64(i), true
	}
	return 0, false
}

func (v Value) Float() (float64, bool) {
	if f, ok := v.raw.(float64); ok {
		return f, true
	}
	if f, ok := v.raw.(float32); ok {
		return float64(f), true
	}
	return 0, false
}

func (v Value) Bool() (bool, bool) {
	if b, ok := v.raw.(bool); ok {
		return b, true
	}
	return false, false
}

func (v Value) Raw() any {
	return v.raw
}

