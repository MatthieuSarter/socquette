package config

import (
	"os"
	"path/filepath"
	"testing"
)

const (
	configName = "socquette.toml"
	workDir    = "/work"
)

func TestResolvePaths(t *testing.T) {
	p := ResolvePaths("", workDir)
	if p.Dir != workDir || p.Config != filepath.Join(workDir, configName) ||
		p.Log != filepath.Join(workDir, "socquette.log") || p.PID != filepath.Join(workDir, "socquette.pid") {
		t.Errorf("cwd paths = %+v", p)
	}
	abs := t.TempDir()
	if p := ResolvePaths(abs, workDir); p.Dir != abs {
		t.Errorf("env paths = %+v", p)
	}
}

func TestLoadMissingFile(t *testing.T) {
	c, exists, err := Load(filepath.Join(t.TempDir(), configName))
	if err != nil || exists {
		t.Fatalf("exists=%v err=%v", exists, err)
	}
	if c.HTTP.Port != Defaults().HTTP.Port {
		t.Errorf("not defaults: %+v", c)
	}
}

func TestSaveThenLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), configName)
	want := Defaults()
	want.HTTP.Enabled = true
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, exists, err := Load(path)
	if err != nil || !exists || !got.HTTP.Enabled {
		t.Fatalf("got=%+v exists=%v err=%v", got, exists, err)
	}
}

func TestSaveIsAtomicAndRejectsInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configName)
	if err := Save(path, Defaults()); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	bad := Defaults()
	bad.HTTP.Port = 0
	if err := Save(path, bad); err == nil {
		t.Fatal("expected a validation error")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Error("file changed after a rejected save")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("leftover files: %v", entries)
	}
}

func TestSaveToMissingDirectory(t *testing.T) {
	if err := Save(filepath.Join(t.TempDir(), "missing", configName), Defaults()); err == nil {
		t.Error("expected an error")
	}
}

func TestLoadInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), configName)
	_ = os.WriteFile(path, []byte("[http]\nport = 0\n"), 0o600)
	if _, exists, err := Load(path); err == nil || !exists {
		t.Errorf("exists=%v err=%v", exists, err)
	}
}

func TestResolvePathsRelativeEnv(t *testing.T) {
	cwd := filepath.Join(t.TempDir(), "work")
	if p := ResolvePaths("conf", cwd); p.Dir != filepath.Join(cwd, "conf") {
		t.Errorf("dir = %q, want it absolute under %q", p.Dir, cwd)
	}
}
