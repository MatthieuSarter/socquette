package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Paths are the files socquette reads and writes, all in one directory.
type Paths struct {
	Dir    string
	Config string
	Log    string
	PID    string
}

// ResolvePaths returns the paths in envConf (the SOCQUETTE_CONF value) when it is set, else in cwd. A relative
// envConf is taken from cwd, so a server started elsewhere (background process, service) finds the same directory.
func ResolvePaths(envConf, cwd string) Paths {
	dir := cwd
	if envConf != "" {
		dir = envConf
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(cwd, dir)
		}
	}
	return Paths{
		Dir:    dir,
		Config: filepath.Join(dir, "socquette.toml"),
		Log:    filepath.Join(dir, "socquette.log"),
		PID:    filepath.Join(dir, "socquette.pid"),
	}
}

// Load reads and parses a configuration file; a missing file yields Defaults() and exists == false.
func Load(path string) (cfg Config, exists bool, err error) {
	data, err := os.ReadFile(path) //nolint:gosec // The path is the configuration file chosen by the operator.
	if errors.Is(err, fs.ErrNotExist) {
		return Defaults(), false, nil
	}
	if err != nil {
		return Config{}, false, fmt.Errorf("read %s: %w", path, err)
	}
	cfg, err = Parse(data)
	if err != nil {
		return Config{}, true, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, true, nil
}

// Save validates a configuration and writes it atomically: a temporary file in the same directory, then a rename,
// so a reader never sees a half-written file.
func Save(path string, c Config) error {
	if err := Validate(c); err != nil {
		return err
	}
	data, err := Marshal(c)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".socquette-*.toml")
	if err != nil {
		return fmt.Errorf("save: %w", err)
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("save: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("save: %w", err)
	}
	return nil
}
