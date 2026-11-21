package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "spotgo")
	path := filepath.Join(dir, "config.json")
	want := Config{Spotify: Credentials{ClientID: "id123", ClientSecret: "sec456"}}

	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Load = %+v, quero %+v", got, want)
	}

	assertPerm(t, path, 0o600)
	assertPerm(t, dir, 0o700)
}

func TestLoadMissingFile(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "nao-existe.json"))
	if err != nil {
		t.Fatalf("arquivo inexistente não deve ser erro: %v", err)
	}
	if got != (Config{}) {
		t.Fatalf("quero Config vazio, veio %+v", got)
	}
}

func TestLoadCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("quero erro para JSON inválido")
	}
	for _, s := range []string{path, "spotgo auth"} {
		if !strings.Contains(err.Error(), s) {
			t.Errorf("erro %q deveria conter %q", err, s)
		}
	}
}

func TestDeleteMissingIsNotError(t *testing.T) {
	if err := Delete(filepath.Join(t.TempDir(), "x.json")); err != nil {
		t.Fatal(err)
	}
}

func assertPerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s: permissão %o, quero %o", path, got, want)
	}
}
