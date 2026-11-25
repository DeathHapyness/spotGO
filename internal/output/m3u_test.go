package output

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteM3U(t *testing.T) {
	dir := t.TempDir()
	m3u := filepath.Join(dir, "Mix.m3u8")
	err := WriteM3U(m3u, []M3UEntry{
		{Path: filepath.Join(dir, "Mix", "01 Um.mp3"), Title: "A - Um", Duration: 61400 * time.Millisecond},
		{Path: filepath.Join(dir, "Dois.mp3"), Title: "B - Dois\nquebra"},
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(m3u)
	want := "#EXTM3U\n#EXTINF:61,A - Um\nMix/01 Um.mp3\n#EXTINF:-1,B - Dois quebra\nDois.mp3\n"
	if string(b) != want {
		t.Errorf("m3u =\n%s\nwant\n%s", b, want)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 1 {
		t.Errorf("sobrou temporário: %v", ents)
	}
}
