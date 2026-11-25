package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DeathHapyness/spotGO/internal/ffmpeg"
	"github.com/DeathHapyness/spotGO/internal/song"
	"github.com/DeathHapyness/spotGO/internal/source"
)

type fakeTagger struct {
	mu    sync.Mutex
	tags  map[string]ffmpeg.Info
	jobs  []ffmpeg.Job
	probe error
}

func (f *fakeTagger) Probe(_ context.Context, path string, _ ffmpeg.Format) (ffmpeg.Info, error) {
	if f.probe != nil {
		return ffmpeg.Info{}, f.probe
	}
	return f.tags[filepath.Base(path)], nil
}

func (f *fakeTagger) Convert(_ context.Context, j ffmpeg.Job) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.jobs = append(f.jobs, j)
	return nil
}

func touch(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, n := range names {
		p := filepath.Join(dir, n)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, nil, 0o644)
	}
}

func runMeta(t *testing.T, tg *fakeTagger, args ...string) (string, string, error) {
	t.Helper()
	r := &fakeResolver{results: map[string]source.Result{
		"2EqlS6tkEnglzr7tkKAAYD": {Songs: []song.Song{{Title: "Come Together", Artists: []string{"The Beatles"}, Album: "Abbey Road", ISRC: "GBAYE0601690"}}},
	}}
	cmd := newMetaCmd(func(func(int, int)) metaEnv {
		return metaEnv{
			resolver: r,
			enrich: func(_ context.Context, s song.Song) (song.Song, bool, error) {
				switch s.Title {
				case "Something":
					s.Album, s.TrackNumber = "Abbey Road", 2
					return s, true, nil
				case "Quebra":
					return s, false, errors.New("Deezer fora do ar")
				}
				return s, false, nil
			},
			covers: noCovers{},
			tagger: func(string) (tagger, error) { return tg, nil },
		}
	})
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	return out.String(), errb.String(), err
}

func TestMeta(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "a.mp3", "The Beatles - Something.opus", "sub/x.flac", "sub/capa.jpg", ".a.mp3.tmp-1.mp3", "quebra.m4a")
	tg := &fakeTagger{tags: map[string]ffmpeg.Info{
		"a.mp3":      {Tags: map[string]string{"title": "ERRADO", "comment": "https://open.spotify.com/track/2EqlS6tkEnglzr7tkKAAYD", "genre": "Rock"}, HasCover: true},
		"x.flac":     {Tags: map[string]string{"title": "Desconhecida", "artist": "Ninguém"}},
		"quebra.m4a": {Tags: map[string]string{"title": "Quebra", "artist": "X", "lyrics": "letra velha"}},
	}}
	out, errOut, err := runMeta(t, tg, "--lyrics", "none", dir)
	if err == nil || !strings.Contains(err.Error(), "1 arquivo falhou") {
		t.Fatalf("err = %v\n%s", err, errOut)
	}
	if !strings.Contains(errOut, "2 atualizados, 1 não achado, 1 com erro") {
		t.Errorf("resumo:\n%s", errOut)
	}
	if !strings.Contains(out, "não achado  "+filepath.Join(dir, "sub", "x.flac")) {
		t.Errorf("saída:\n%s", out)
	}
	if len(tg.jobs) != 2 {
		t.Fatalf("jobs: %+v", tg.jobs)
	}
	for _, j := range tg.jobs {
		if !j.Copy || j.Input != j.Output {
			t.Errorf("tem que copiar o áudio no mesmo arquivo: %+v", j)
		}
		switch filepath.Base(j.Input) {
		case "a.mp3":
			if j.Song.Title != "Come Together" || j.Song.ISRC == "" || !j.KeepCover || j.Extra["genre"] != "Rock" || j.Format != ffmpeg.MP3 {
				t.Errorf("a.mp3: %+v", j)
			}
		case "The Beatles - Something.opus":
			if j.Song.Title != "Something" || j.Song.MainArtist() != "The Beatles" || j.Song.TrackNumber != 2 || j.Format != ffmpeg.Opus {
				t.Errorf("pelo nome do arquivo: %+v", j.Song)
			}
		}
	}
}

func TestMetaDryRunAndKeepsLyrics(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "a.mp3")
	tg := &fakeTagger{tags: map[string]ffmpeg.Info{
		"a.mp3": {Tags: map[string]string{"title": "Something", "artist": "The Beatles", "lyrics": "letra velha"}},
	}}
	if _, _, err := runMeta(t, tg, "--dry-run", dir); err != nil || len(tg.jobs) != 0 {
		t.Fatalf("--dry-run gravou: %v %v", err, tg.jobs)
	}
	if _, _, err := runMeta(t, tg, "--lyrics", "none", dir); err != nil {
		t.Fatal(err)
	}
	if tg.jobs[0].Lyrics != "letra velha" {
		t.Errorf("sem letra nova, a antiga fica: %q", tg.jobs[0].Lyrics)
	}
}

func TestMetaErrors(t *testing.T) {
	if _, _, err := runMeta(t, &fakeTagger{}, filepath.Join(t.TempDir(), "nada.mp3")); err == nil {
		t.Error("arquivo que não existe")
	}
	dir := t.TempDir()
	touch(t, dir, "capa.jpg")
	if _, _, err := runMeta(t, &fakeTagger{}, dir); err == nil || !strings.Contains(err.Error(), "nenhum arquivo") {
		t.Errorf("pasta sem áudio: %v", err)
	}
	if _, _, err := runMeta(t, &fakeTagger{}, filepath.Join(dir, "capa.jpg")); err == nil {
		t.Error("arquivo que não é áudio")
	}
}

func TestSongFromTags(t *testing.T) {
	s := songFromTags(ffmpeg.Info{Duration: time.Minute, Tags: map[string]string{
		"title": " T ", "artist": "AC/DC; Brian Johnson, X", "comment": "veja https://open.spotify.com/intl-pt/track/2EqlS6tkEnglzr7tkKAAYD?si=1",
	}}, "/m/qualquer.mp3")
	if s.Title != "T" || strings.Join(s.Artists, "|") != "AC/DC|Brian Johnson|X" || s.SpotifyID != "2EqlS6tkEnglzr7tkKAAYD" || s.Duration != time.Minute {
		t.Errorf("songFromTags = %+v", s)
	}
	s = songFromTags(ffmpeg.Info{Tags: map[string]string{}}, "/m/Artista - Título - Live.mp3")
	if s.Title != "Título - Live" || s.MainArtist() != "Artista" {
		t.Errorf("pelo nome: %+v", s)
	}
	s = songFromTags(ffmpeg.Info{Tags: map[string]string{}}, "/m/só título.mp3")
	if s.Title != "só título" || len(s.Artists) != 0 {
		t.Errorf("sem artista: %+v", s)
	}
}
