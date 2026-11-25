package download

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DeathHapyness/spotGO/internal/ffmpeg"
	"github.com/DeathHapyness/spotGO/internal/matching"
	"github.com/DeathHapyness/spotGO/internal/output"
	"github.com/DeathHapyness/spotGO/internal/song"
)

var errNotFound = errors.New("não achei")

type fakeFinder struct {
	missing map[string]bool
	broken  map[string]bool
}

func (f *fakeFinder) Find(ctx context.Context, s song.Song) (matching.Scored, error) {
	switch {
	case f.missing[s.Title]:
		return matching.Scored{}, errNotFound
	case f.broken[s.Title]:
		return matching.Scored{}, errors.New("rede caiu")
	}
	return matching.Scored{Result: matching.Result{URL: "https://yt/" + s.Title}}, nil
}

type fakeFetcher struct {
	fail    map[string]bool
	block   chan struct{}
	active  atomic.Int32
	maxSeen atomic.Int32
	dirs    sync.Map
}

func (f *fakeFetcher) Fetch(ctx context.Context, url string, _ ffmpeg.Format, dir string) (string, string, error) {
	n := f.active.Add(1)
	defer f.active.Add(-1)
	for {
		m := f.maxSeen.Load()
		if n <= m || f.maxSeen.CompareAndSwap(m, n) {
			break
		}
	}
	f.dirs.Store(dir, true)
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return "", "", ctx.Err()
		}
	}
	if f.fail[strings.TrimPrefix(url, "https://yt/")] {
		return "", "", errors.New("yt-dlp falhou")
	}
	file := filepath.Join(dir, "a.webm")
	return file, "opus", os.WriteFile(file, []byte("áudio"), 0o644)
}

type fakeConverter struct {
	mu   sync.Mutex
	jobs []ffmpeg.Job
}

func (c *fakeConverter) Convert(ctx context.Context, j ffmpeg.Job) error {
	c.mu.Lock()
	c.jobs = append(c.jobs, j)
	c.mu.Unlock()
	os.MkdirAll(filepath.Dir(j.Output), 0o755)
	return os.WriteFile(j.Output, []byte("final"), 0o644)
}

type fakeCovers struct{ err error }

func (c fakeCovers) Cover(ctx context.Context, url string) ([]byte, error) {
	return []byte("capa:" + url), c.err
}

func items(titles ...string) []Item {
	var out []Item
	for _, t := range titles {
		out = append(out, Item{Song: song.Song{Title: t, Artists: []string{"Art"}, CoverURL: "c/" + t}})
	}
	return out
}

func newPipeline(t *testing.T, f *fakeFinder, fe *fakeFetcher, c *fakeConverter) *Pipeline {
	t.Helper()
	tmpl, err := output.Parse(output.DefaultTemplate)
	if err != nil {
		t.Fatal(err)
	}
	return &Pipeline{
		Finder: f, Fetcher: fe, Converter: c, Covers: fakeCovers{},
		Options: Options{Dir: t.TempDir(), Template: tmpl, Format: ffmpeg.MP3, ErrNotFound: errNotFound},
	}
}

func statuses(out []Outcome) []Status {
	s := make([]Status, len(out))
	for i, o := range out {
		s[i] = o.Status
	}
	return s
}

func TestRunMixed(t *testing.T) {
	fe := &fakeFetcher{fail: map[string]bool{"d": true}}
	c := &fakeConverter{}
	p := newPipeline(t, &fakeFinder{missing: map[string]bool{"b": true}, broken: map[string]bool{"c": true}}, fe, c)
	var done []int
	p.OnDone = func(i int, o Outcome) { done = append(done, i) }

	out := p.Run(context.Background(), items("a", "b", "c", "d", "e"))

	want := []Status{Downloaded, NotFound, Failed, Failed, Downloaded}
	if got := statuses(out); !slices.Equal(got, want) {
		t.Fatalf("status = %v, want %v", got, want)
	}
	if out[0].Path != filepath.Join(p.Dir, "Art - a.mp3") || out[0].URL != "https://yt/a" {
		t.Errorf("outcome: %+v", out[0])
	}
	if _, err := os.Stat(out[4].Path); err != nil {
		t.Error(err)
	}
	if len(done) != 5 {
		t.Errorf("OnDone chamado %d vezes", len(done))
	}
	for _, j := range c.jobs {
		if j.InputCodec != "opus" || string(j.Cover) != "capa:c/"+j.Song.Title || j.Format != ffmpeg.MP3 {
			t.Errorf("job: %+v", j)
		}
	}
	fe.dirs.Range(func(k, _ any) bool {
		if _, err := os.Stat(k.(string)); !os.IsNotExist(err) {
			t.Errorf("pasta temporária não foi apagada: %s", k)
		}
		return true
	})
}

func TestRunSkipsExistingAndDuplicates(t *testing.T) {
	c := &fakeConverter{}
	p := newPipeline(t, &fakeFinder{}, &fakeFetcher{}, c)
	os.WriteFile(filepath.Join(p.Dir, "Art - a.mp3"), []byte("já tenho"), 0o644)

	out := p.Run(context.Background(), items("a", "b", "b"))

	if got := statuses(out); !slices.Equal(got, []Status{Skipped, Downloaded, Skipped}) {
		t.Fatalf("status = %v", got)
	}
	if len(c.jobs) != 1 {
		t.Errorf("convertidas: %d", len(c.jobs))
	}

	p.Overwrite = true
	out = p.Run(context.Background(), items("a"))
	if out[0].Status != Downloaded {
		t.Errorf("com Overwrite: %v", out[0].Status)
	}
}

func TestRunCoverErrorIsNotFatal(t *testing.T) {
	c := &fakeConverter{}
	p := newPipeline(t, &fakeFinder{}, &fakeFetcher{}, c)
	p.Covers = fakeCovers{err: errors.New("404")}
	out := p.Run(context.Background(), items("a"))
	if out[0].Status != Downloaded {
		t.Fatalf("status = %v (%v)", out[0].Status, out[0].Err)
	}
}

func TestRunConcurrencyLimit(t *testing.T) {
	fe := &fakeFetcher{block: make(chan struct{})}
	p := newPipeline(t, &fakeFinder{}, fe, &fakeConverter{})
	p.DownloadWorkers = 2
	go func() {
		time.Sleep(50 * time.Millisecond)
		close(fe.block)
	}()
	out := p.Run(context.Background(), items("a", "b", "c", "d", "e", "f"))
	if got := fe.maxSeen.Load(); got != 2 {
		t.Errorf("downloads simultâneos = %d, want 2", got)
	}
	for _, o := range out {
		if o.Status != Downloaded {
			t.Errorf("%s: %v", o.Song.Title, o.Status)
		}
	}
}

func TestRunCanceled(t *testing.T) {
	fe := &fakeFetcher{block: make(chan struct{})}
	p := newPipeline(t, &fakeFinder{}, fe, &fakeConverter{})
	p.DownloadWorkers = 1
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for fe.active.Load() == 0 {
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()

	finished := make(chan []Outcome)
	go func() { finished <- p.Run(ctx, items("a", "b", "c", "d", "e", "f", "g", "h")) }()
	select {
	case out := <-finished:
		for _, o := range out {
			if o.Status != Canceled || !errors.Is(o.Err, context.Canceled) {
				t.Errorf("%s: %v %v", o.Song.Title, o.Status, o.Err)
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run não terminou depois do cancelamento")
	}
	if ents, _ := os.ReadDir(p.Dir); len(ents) != 0 {
		t.Errorf("sobrou arquivo: %v", ents)
	}
}

func TestRunListVars(t *testing.T) {
	c := &fakeConverter{}
	p := newPipeline(t, &fakeFinder{}, &fakeFetcher{}, c)
	p.Template, _ = output.Parse("{list-name}/{list-position} {title}")
	in := items("a")
	in[0].ListName, in[0].ListPosition, in[0].ListLength = "Mix", 3, 12
	out := p.Run(context.Background(), in)
	if want := filepath.Join(p.Dir, "Mix", "03 a.mp3"); out[0].Path != want {
		t.Errorf("Path = %q, want %q", out[0].Path, want)
	}
}

func TestYTDLPArgs(t *testing.T) {
	y := &YTDLP{Deno: "/bin/deno"}
	a := y.Args("https://music.youtube.com/watch?v=x", ffmpeg.Opus, "/tmp/d")
	s := strings.Join(a, " ")
	for _, want := range []string{"--ignore-config", "bestaudio[acodec=opus]", "--js-runtimes deno:/bin/deno", "/tmp/d/%(id)s.%(ext)s"} {
		if !strings.Contains(s, want) {
			t.Errorf("faltou %q em %s", want, s)
		}
	}
	if a[len(a)-2] != "--" {
		t.Errorf("a URL deve vir depois de --: %v", a)
	}
	if s := strings.Join((&YTDLP{}).Args("u", ffmpeg.MP3, "d"), " "); strings.Contains(s, "js-runtimes") || !strings.Contains(s, "-f bestaudio/best") {
		t.Errorf("mp3 sem deno: %s", s)
	}
}

func TestYTDLPFetch(t *testing.T) {
	y := &YTDLP{Path: "yt-dlp", Run: func(ctx context.Context, path string, args []string) ([]byte, []byte, error) {
		return []byte("[info] algo\n/tmp/d/x.webm\topus\n"), nil, nil
	}}
	file, codec, err := y.Fetch(context.Background(), "u", ffmpeg.Opus, "/tmp/d")
	if err != nil || file != "/tmp/d/x.webm" || codec != "opus" {
		t.Errorf("Fetch = %q, %q, %v", file, codec, err)
	}

	y.Run = func(ctx context.Context, path string, args []string) ([]byte, []byte, error) {
		return nil, []byte("WARNING: x\nERROR: [youtube] x: Video unavailable\n"), errors.New("saiu com código 1")
	}
	if _, _, err := y.Fetch(context.Background(), "u", ffmpeg.Opus, "/tmp/d"); err == nil || !strings.Contains(err.Error(), "Video unavailable") {
		t.Errorf("err = %v", err)
	}

	y.Run = func(ctx context.Context, path string, args []string) ([]byte, []byte, error) { return nil, nil, nil }
	if _, _, err := y.Fetch(context.Background(), "u", ffmpeg.Opus, "/tmp/d"); err == nil {
		t.Error("saída vazia deve dar erro")
	}
}
