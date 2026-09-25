// Package config loads and saves the small amount of state thock remembers
// between runs: the chosen theme and the last test settings.
package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is the persisted user preference set.
type Config struct {
	Theme string `toml:"theme"`
	Test  Test   `toml:"test"`
	Race  Race   `toml:"race"`
}

// Race is what thock remembers about racing.
type Race struct {
	Name string `toml:"name"` // shown to the other racers
}

// Test is the last-used test setup, restored on the next bare run.
type Test struct {
	Mode     string `toml:"mode"`     // time | words | quotes
	Duration int    `toml:"duration"` // seconds, time mode
	Words    int    `toml:"words"`    // word count, words mode
	List     string `toml:"list"`     // 1k | 5k
	Length   string `toml:"length"`   // quote bucket
}

// Default is the configuration used before the user has changed anything.
func Default() Config {
	return Config{
		Theme: "mono",
		Test: Test{
			Mode:     "time",
			Duration: 30,
			Words:    25,
			List:     "1k",
			Length:   "any",
		},
	}
}

// migrate repairs configurations written by an earlier version. "words" once
// meant a timed test; it now means a word-count test, so a saved mode of
// "words" carrying no word count is a timed setup and is renamed rather than
// silently changing what a bare run does. It operates on the decoded file
// alone, where a zero field genuinely means the key was absent.
func migrate(raw Config) Config {
	if raw.Test.Mode == "words" && raw.Test.Words == 0 {
		raw.Test.Mode = "time"
	}
	return raw
}

// merge layers a decoded file over the defaults, so a missing or unusable
// value falls back rather than failing. Preferences should never be able to
// lock someone out of their own typing test.
func merge(def, raw Config) Config {
	out := def
	if raw.Theme != "" {
		out.Theme = raw.Theme
	}
	switch raw.Test.Mode {
	case "time", "words", "quotes":
		out.Test.Mode = raw.Test.Mode
	}
	if raw.Test.Duration > 0 {
		out.Test.Duration = raw.Test.Duration
	}
	if raw.Test.Words > 0 {
		out.Test.Words = raw.Test.Words
	}
	if raw.Test.List != "" {
		out.Test.List = raw.Test.List
	}
	if raw.Test.Length != "" {
		out.Test.Length = raw.Test.Length
	}
	if raw.Race.Name != "" {
		out.Race.Name = raw.Race.Name
	}
	return out
}

// Dir is the directory holding config.toml, honouring XDG_CONFIG_HOME and
// otherwise using ~/.config, which is where terminal tools are looked for even
// on macOS.
func Dir() (string, error) {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "thock"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "thock"), nil
}

// Path is the full path to config.toml.
func Path() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.toml"), nil
}

// Load reads the config, returning defaults when the file does not exist yet.
// A malformed file is also reported as defaults plus an error, so the caller
// can warn without preventing the user from typing.
func Load() (Config, error) {
	c := Default()
	p, err := Path()
	if err != nil {
		return c, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	// Decoded into a zero value, not over the defaults, so that an absent key
	// is distinguishable from one explicitly set to its default.
	var raw Config
	if err := toml.Unmarshal(b, &raw); err != nil {
		return c, err
	}
	return merge(c, migrate(raw)), nil
}

// Save writes the config, creating the directory if needed.
func Save(c Config) error {
	d, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	p := filepath.Join(d, "config.toml")
	f, err := os.CreateTemp(d, ".config-*.toml")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)

	if err := toml.NewEncoder(f).Encode(c); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
