package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/DeathHapyness/spotGO/internal/deps"
	"github.com/DeathHapyness/spotGO/internal/download"
	"github.com/DeathHapyness/spotGO/internal/song"
	"github.com/DeathHapyness/spotGO/internal/source"
	"github.com/DeathHapyness/spotGO/internal/syncstate"
)

func playlist(titles ...string) source.Result {
	r := source.Result{Name: "Mix"}
	for _, t := range titles {
		r.Songs = append(r.Songs, song.Song{Title: t, Artists: []string{"A"}, SpotifyID: "id-" + t})
	}
	return r
}

type syncRig struct {
	t    *testing.T
	dir  string
	r    *fakeResolver
	conv *fakeConv
}

func newSyncRig(t *testing.T) *syncRig {
	return &syncRig{t: t, dir: t.TempDir(), r: &fakeResolver{results: map[string]source.Result{}, errs: map[string]error{}}}
}

func (g *syncRig) run(args ...string) (string, string, error) {
	g.t.Helper()
	g.conv = &fakeConv{}
	cmd := newSyncCmd(func(func(int, int)) downloadEnv {
		return downloadEnv{
			resolver: g.r, finder: findAllOK, covers: noCovers{},
			tools: func(context.Context, downloadFlags, func([]deps.Tool) error) (download.Fetcher, download.Converter, []string, error) {
				return fakeFetch{}, g.conv, nil, nil
			},
		}
	})
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(append([]string{"--dir", g.dir}, args...))
	err := cmd.ExecuteContext(context.Background())
	return out.String(), errb.String(), err
}

func (g *syncRig) files() []string {
	var out []string
	filepath.Walk(g.dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && info.Name() != syncstate.FileName {
			rel, _ := filepath.Rel(g.dir, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(out)
	return out
}

func playlistURL(id string) string { return "https://open.spotify.com/playlist/" + id }

func TestSyncLifecycle(t *testing.T) {
	g := newSyncRig(t)
	g.r.results[idBom] = playlist("Um", "Dois", "Três")
	if _, errOut, err := g.run("-o", "{list-position} {title}", "--format", "opus", playlistURL(idBom)); err != nil {
		t.Fatal(err, errOut)
	}
	if got := strings.Join(g.files(), ","); got != "1 Um.opus,2 Dois.opus,3 Três.opus" {
		t.Fatalf("1ª rodada: %s", got)
	}
	// Um arquivo do usuário, que o sync nunca deve apagar.
	os.WriteFile(filepath.Join(g.dir, "minha.opus"), nil, 0o644)

	// Dois saiu, Quatro entrou no começo: Um e Três mudam de posição.
	g.r.results[idBom] = playlist("Quatro", "Um", "Três")
	out, errOut, err := g.run()
	if err != nil {
		t.Fatal(err, errOut)
	}
	if got := strings.Join(g.files(), ","); got != "1 Quatro.opus,2 Um.opus,3 Três.opus,minha.opus" {
		t.Fatalf("2ª rodada: %s\n%s", got, out)
	}
	if len(g.conv.jobs) != 1 || g.conv.jobs[0].Song.Title != "Quatro" {
		t.Errorf("só o Quatro devia ser baixado: %+v", g.conv.jobs)
	}
	for _, want := range []string{"movido      1 Um.opus  →  2 Um.opus", "apagado     2 Dois.opus"} {
		if !strings.Contains(out, want) {
			t.Errorf("faltou %q em:\n%s", want, out)
		}
	}

	st, _, _ := syncstate.Load(g.dir)
	if st.Format != "opus" || st.Template != "{list-position} {title}" || len(st.Files) != 3 || st.Files["id-Um"] != "2 Um.opus" {
		t.Errorf("estado: %+v", st)
	}
}

func TestSyncFormatChange(t *testing.T) {
	g := newSyncRig(t)
	g.r.results[idBom] = playlist("Um")
	g.run(playlistURL(idBom))
	if _, _, err := g.run("--format", "flac"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(g.files(), ","); got != "A - Um.flac" {
		t.Errorf("files: %s", got)
	}
}

func TestSyncLinkErrorDeletesNothing(t *testing.T) {
	g := newSyncRig(t)
	g.r.results[idBom] = playlist("Um")
	g.r.results[id404] = playlist("Dois")
	g.run(playlistURL(idBom), playlistURL(id404))
	g.r.errs[id404] = errors.New("embed quebrou")

	_, errOut, err := g.run()
	if err == nil || !strings.Contains(errOut, "nada foi apagado") {
		t.Fatalf("err=%v\n%s", err, errOut)
	}
	if got := strings.Join(g.files(), ","); got != "A - Dois.mp3,A - Um.mp3" {
		t.Errorf("files: %s", got)
	}
	delete(g.r.errs, id404)
	g.r.results[id404] = playlist()
	if _, _, err := g.run(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(g.files(), ","); got != "A - Um.mp3" {
		t.Errorf("depois que o link volta, a faixa removida sai: %s", got)
	}
}

func TestSyncKeep(t *testing.T) {
	g := newSyncRig(t)
	g.r.results[idBom] = playlist("Um", "Dois")
	g.run(playlistURL(idBom))
	g.r.results[idBom] = playlist("Um")
	g.run("--keep")
	if got := len(g.files()); got != 2 {
		t.Errorf("--keep apagou: %v", g.files())
	}
	g.run()
	if got := strings.Join(g.files(), ","); got != "A - Um.mp3" {
		t.Errorf("sem --keep, a faixa sai: %s", got)
	}
}

func TestSyncNeedsLinksFirstTime(t *testing.T) {
	g := newSyncRig(t)
	if _, _, err := g.run(); err == nil || !strings.Contains(err.Error(), "spotgo sync <link>") {
		t.Errorf("err = %v", err)
	}
}

func TestDownloadArchive(t *testing.T) {
	conv := &fakeConv{}
	arch := filepath.Join(t.TempDir(), "a.txt")
	os.WriteFile(arch, []byte("spotify id-Um\n"), 0o644)
	r := &fakeResolver{results: map[string]source.Result{idBom: playlist("Um", "Dois")}}
	run := func() string {
		cmd := newDownloadCmd(func(func(int, int)) downloadEnv {
			return downloadEnv{resolver: r, finder: findAllOK, covers: noCovers{},
				tools: func(context.Context, downloadFlags, func([]deps.Tool) error) (download.Fetcher, download.Converter, []string, error) {
					return fakeFetch{}, conv, nil, nil
				}}
		})
		var out, errb bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errb)
		dir := t.TempDir()
		cmd.SetArgs([]string{"--archive", arch, "-o", dir + "/{title}", playlistURL(idBom)})
		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatal(err)
		}
		return errb.String()
	}
	errOut := run()
	if len(conv.jobs) != 1 || conv.jobs[0].Song.Title != "Dois" || !strings.Contains(errOut, "1 pulada pelo --archive") {
		t.Fatalf("jobs=%+v\n%s", conv.jobs, errOut)
	}
	b, _ := os.ReadFile(arch)
	if string(b) != "spotify id-Um\nspotify id-Dois\n" {
		t.Errorf("archive = %q", b)
	}
	run()
	if len(conv.jobs) != 1 {
		t.Errorf("a 2ª rodada não deveria baixar nada: %d", len(conv.jobs))
	}
}

func TestSyncM3UAndLyricsSaved(t *testing.T) {
	g := newSyncRig(t)
	g.r.results[idBom] = playlist("Um", "Dois")
	if _, errOut, err := g.run("--m3u", "--lyrics", "tag,lrc", "-o", "{list-name}/{title}", playlistURL(idBom)); err != nil {
		t.Fatal(err, errOut)
	}
	b, err := os.ReadFile(filepath.Join(g.dir, "Mix.m3u8"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "#EXTM3U\n#EXTINF:-1,A - Um\nMix/Um.mp3\n#EXTINF:-1,A - Dois\nMix/Dois.mp3\n"; string(b) != want {
		t.Errorf("m3u:\n%s", b)
	}
	st, _, _ := syncstate.Load(g.dir)
	if !st.M3U || st.Lyrics != "tag,lrc" {
		t.Errorf("estado: %+v", st)
	}

	// Dois sai da playlist: some do m3u também.
	g.r.results[idBom] = playlist("Um")
	g.run()
	b, _ = os.ReadFile(filepath.Join(g.dir, "Mix.m3u8"))
	if strings.Contains(string(b), "Dois") {
		t.Errorf("m3u ainda tem a faixa removida:\n%s", b)
	}
}

func TestDownloadBadLyrics(t *testing.T) {
	if _, _, err := runDownload(t, fakeFetch{}, &fakeConv{}, findAllOK, nil, "--lyrics", "genius", albumURL(idBom)); err == nil {
		t.Error("--lyrics inválido deve falhar")
	}
}
