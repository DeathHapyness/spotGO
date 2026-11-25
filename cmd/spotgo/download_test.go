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

	"github.com/DeathHapyness/spotGO/internal/deps"
	"github.com/DeathHapyness/spotGO/internal/download"
	"github.com/DeathHapyness/spotGO/internal/ffmpeg"
	"github.com/DeathHapyness/spotGO/internal/matching"
	"github.com/DeathHapyness/spotGO/internal/provider/ytmusic"
	"github.com/DeathHapyness/spotGO/internal/song"
	"github.com/DeathHapyness/spotGO/internal/source"
)

type fakeFetch struct{ fail string }

func (f fakeFetch) Fetch(ctx context.Context, url string, _ ffmpeg.Format, dir string) (string, string, error) {
	if strings.HasSuffix(url, f.fail) && f.fail != "" {
		return "", "", errors.New("Video unavailable")
	}
	p := filepath.Join(dir, "a.webm")
	return p, "opus", os.WriteFile(p, nil, 0o644)
}

type fakeConv struct {
	mu   sync.Mutex
	jobs []ffmpeg.Job
}

func (c *fakeConv) Convert(ctx context.Context, j ffmpeg.Job) error {
	c.mu.Lock()
	c.jobs = append(c.jobs, j)
	c.mu.Unlock()
	os.MkdirAll(filepath.Dir(j.Output), 0o755)
	return os.WriteFile(j.Output, nil, 0o644)
}

type noCovers struct{}

func (noCovers) Cover(context.Context, string) ([]byte, error) { return nil, nil }

func runDownload(t *testing.T, fetch fakeFetch, conv *fakeConv, finder finder, toolErr error, args ...string) (string, string, error) {
	t.Helper()
	dir := t.TempDir()
	wd, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(wd) })

	r := &fakeResolver{results: map[string]source.Result{idBom: threeSongs()}}
	cmd := newDownloadCmd(func(func(int, int)) downloadEnv {
		return downloadEnv{
			resolver: r, finder: finder, covers: noCovers{},
			tools: func(ctx context.Context, f downloadFlags, confirm func([]deps.Tool) error) (download.Fetcher, download.Converter, []string, error) {
				if toolErr != nil {
					return nil, nil, nil, toolErr
				}
				return fetch, conv, []string{"o yt-dlp tem 90 dias"}, nil
			},
		}
	})
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	return out.String(), errb.String(), err
}

var findAllOK = fakeFinder{func(_ context.Context, s song.Song) (matching.Scored, error) { return found(s) }}

func TestDownloadAlbum(t *testing.T) {
	conv := &fakeConv{}
	out, errOut, err := runDownload(t, fakeFetch{}, conv, findAllOK, nil,
		"--format", "flac", "-o", "{list-name}/{list-position} {title}", albumURL(idBom))
	if err != nil {
		t.Fatal(err, errOut)
	}
	if !strings.Contains(out, "baixado     A - Um  →  "+filepath.Join("Álbum", "1 Um.flac")) {
		t.Errorf("saída:\n%s", out)
	}
	if !strings.Contains(errOut, "3 baixadas") || !strings.Contains(errOut, "aviso: o yt-dlp tem 90 dias") {
		t.Errorf("stderr:\n%s", errOut)
	}
	if len(conv.jobs) != 3 || conv.jobs[0].Format != ffmpeg.FLAC {
		t.Errorf("jobs: %+v", conv.jobs)
	}
}

func TestDownloadNotFoundAndFailure(t *testing.T) {
	f := fakeFinder{func(_ context.Context, s song.Song) (matching.Scored, error) {
		if s.Title == "Dois" {
			return matching.Scored{}, ytmusic.ErrNotFound
		}
		return found(s)
	}}
	out, errOut, err := runDownload(t, fakeFetch{fail: "Três"}, &fakeConv{}, f, nil, albumURL(idBom))
	if err == nil || !strings.Contains(err.Error(), "1 faixas falharam") {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out, "não achada  B, C - Dois") || !strings.Contains(errOut, "erro        D - Três: Video unavailable") {
		t.Errorf("out:\n%s\nerr:\n%s", out, errOut)
	}
	if !strings.Contains(errOut, "1 baixada, 1 não achada no YouTube Music, 1 com erro") {
		t.Errorf("resumo:\n%s", errOut)
	}
}

func TestDownloadBadFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--format", "aiff", albumURL(idBom)},
		{"-o", "{titel}", albumURL(idBom)},
		{"--threads", "0", albumURL(idBom)},
	} {
		if _, _, err := runDownload(t, fakeFetch{}, &fakeConv{}, findAllOK, nil, args...); err == nil {
			t.Errorf("%v: esperava erro", args)
		}
	}
}

func TestDownloadToolsMissing(t *testing.T) {
	_, _, err := runDownload(t, fakeFetch{}, &fakeConv{}, findAllOK, errors.New("faltam ffmpeg; rode spotgo setup"), albumURL(idBom))
	if err == nil || !strings.Contains(err.Error(), "spotgo setup") {
		t.Errorf("err = %v", err)
	}
}

func TestEnsureTools(t *testing.T) {
	dir := t.TempDir()
	// O deno está no "PATH"; o yt-dlp e o ffmpeg faltam.
	loc := &deps.Locator{
		Dir: dir,
		LookPath: func(name string) (string, error) {
			if name == "deno" {
				return "/usr/bin/deno", nil
			}
			return "", errors.New("não está")
		},
		Run: func(ctx context.Context, path string, args ...string) (string, error) {
			if strings.HasSuffix(path, "yt-dlp") {
				return "2026.01.01\n", nil
			}
			return "", errors.New("sem versão")
		},
	}
	var installed []deps.Tool
	install := func(ctx context.Context, tl deps.Tool) (string, error) {
		installed = append(installed, tl)
		return filepath.Join(dir, string(tl)), nil
	}
	var asked []deps.Tool
	now := time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)

	found, warns, err := ensureTools(context.Background(), loc, install, func(m []deps.Tool) error { asked = m; return nil }, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(asked) != 2 || len(installed) != 2 || found[deps.Deno].Path != "/usr/bin/deno" {
		t.Errorf("asked=%v installed=%v found=%v", asked, installed, found)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "setup --update") {
		t.Errorf("warnings: %v", warns)
	}

	installed = nil
	_, _, err = ensureTools(context.Background(), loc, install, func([]deps.Tool) error { return errors.New("recusou") }, now)
	if err == nil || len(installed) != 0 {
		t.Errorf("recusar não deve instalar: %v %v", err, installed)
	}

	_, _, err = ensureTools(context.Background(), loc, func(context.Context, deps.Tool) (string, error) {
		return "", deps.ErrUnsupported
	}, func([]deps.Tool) error { return nil }, now)
	if !errors.Is(err, deps.ErrUnsupported) {
		t.Errorf("err = %v", err)
	}
}
