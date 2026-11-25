package syncstate

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		return "<sem arquivo>"
	}
	return string(b)
}

func TestLoadSave(t *testing.T) {
	dir := t.TempDir()
	s, ok, err := Load(dir)
	if err != nil || ok || s.Files == nil {
		t.Fatalf("Load vazio = %+v %v %v", s, ok, err)
	}
	s = State{Links: []string{"l"}, Template: "{title}", Format: "mp3", Files: map[string]string{"a": "x/a.mp3"}}
	if err := Save(dir, s); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Load(dir)
	if err != nil || !ok || got.Files["a"] != "x/a.mp3" || got.Links[0] != "l" {
		t.Fatalf("Load = %+v %v %v", got, ok, err)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 1 {
		t.Errorf("sobrou temporário: %v", ents)
	}
}

func TestLoadRejectsUnsafePaths(t *testing.T) {
	for _, p := range []string{"../fora.mp3", "/etc/passwd", "a/../../b", "", "."} {
		dir := t.TempDir()
		Save(dir, State{Files: map[string]string{"x": p}})
		if _, _, err := Load(dir); err == nil {
			t.Errorf("%q: esperava erro", p)
		}
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, FileName), []byte("{"), 0o644)
	if _, _, err := Load(dir); err == nil {
		t.Error("JSON quebrado deve dar erro")
	}
}

func TestMoves(t *testing.T) {
	old := map[string]string{"a": "01 a.mp3", "b": "02 b.mp3", "c": "c.mp3", "d": "d.mp3"}
	cur := map[string]string{"a": "02 a.mp3", "b": "03 b.mp3", "c": "c.opus", "d": "d.mp3"}
	got := Moves(old, cur)
	want := []Move{{"01 a.mp3", "02 a.mp3"}, {"02 b.mp3", "03 b.mp3"}}
	if !slices.Equal(got, want) {
		t.Errorf("Moves = %v, want %v", got, want)
	}
}

func TestApplyMovesChain(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "L/01 a.mp3", "a")
	write(t, dir, "L/02 b.mp3", "b")
	write(t, dir, "velha/c.mp3", "c")
	write(t, dir, "ocupado.mp3", "outro")
	write(t, dir, "e.mp3", "e")
	moves := []Move{
		{"L/01 a.mp3", "L/02 a.mp3"},
		{"L/02 b.mp3", "L/03 b.mp3"},
		{"velha/c.mp3", "nova/c.mp3"},
		{"sumiu.mp3", "x.mp3"},
		{"e.mp3", "ocupado.mp3"},
	}
	done, err := ApplyMoves(dir, moves)
	if err != nil {
		t.Fatal(err)
	}
	if len(done) != 3 {
		t.Errorf("done = %v", done)
	}
	for rel, want := range map[string]string{
		"L/02 a.mp3": "a", "L/03 b.mp3": "b", "nova/c.mp3": "c",
		"ocupado.mp3": "outro", "e.mp3": "e", "L/01 a.mp3": "<sem arquivo>",
	} {
		if got := read(t, dir, rel); got != want {
			t.Errorf("%s = %q, want %q", rel, got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "velha")); !os.IsNotExist(err) {
		t.Error("a pasta vazia deveria ter sido apagada")
	}
}

func TestStale(t *testing.T) {
	old := map[string]string{"saiu": "s.mp3", "fica": "f.mp3", "formato": "x.mp3", "falhou": "y.mp3", "dup": "d.mp3"}
	cur := map[string]string{"fica": "f.mp3", "formato": "x.opus", "falhou": "y.opus", "outra": "d.mp3"}
	ok := map[string]bool{"fica": true, "formato": true}
	got := Stale(old, cur, ok)
	if want := []string{"s.mp3", "x.mp3"}; !slices.Equal(got, want) {
		t.Errorf("Stale = %v, want %v", got, want)
	}
}

func TestArchive(t *testing.T) {
	p := filepath.Join(t.TempDir(), "archive.txt")
	a, err := OpenArchive(p)
	if err != nil || a.Has("x") {
		t.Fatal(err)
	}
	a.Add("x")
	a.Add("x")
	a.Add("")
	b, err := OpenArchive(p)
	if err != nil || !b.Has("x") || b.Has("y") {
		t.Fatalf("reaberto: %v", err)
	}
	if got := read(t, filepath.Dir(p), "archive.txt"); got != "spotify x\n" {
		t.Errorf("conteúdo = %q", got)
	}
}

func TestSidecars(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "L/01 a.mp3", "a")
	write(t, dir, "L/01 a.lrc", "letra a")
	write(t, dir, "L/02 b.mp3", "b")
	write(t, dir, "L/02 b.lrc", "letra b")
	done, err := ApplyMoves(dir, []Move{{"L/01 a.mp3", "L/02 a.mp3"}, {"L/02 b.mp3", "L/03 b.mp3"}})
	if err != nil || len(done) != 2 {
		t.Fatalf("done=%v err=%v", done, err)
	}
	if read(t, dir, "L/02 a.lrc") != "letra a" || read(t, dir, "L/03 b.lrc") != "letra b" {
		t.Error("o .lrc não acompanhou o áudio")
	}

	removed, err := Remove(dir, "L/02 a.mp3")
	if err != nil || !removed || read(t, dir, "L/02 a.lrc") != "<sem arquivo>" {
		t.Errorf("Remove: %v %v", removed, err)
	}
	removed, err = Remove(dir, "L/sumiu.mp3")
	if err != nil || removed {
		t.Errorf("arquivo que não existe: %v %v", removed, err)
	}
}
