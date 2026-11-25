// Package download liga os estágios busca → download → conversão em pools de
// goroutines, cada um com a sua concorrência (rede ≠ CPU).
package download

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/DeathHapyness/spotGO/internal/ffmpeg"
	"github.com/DeathHapyness/spotGO/internal/lyrics"
	"github.com/DeathHapyness/spotGO/internal/matching"
	"github.com/DeathHapyness/spotGO/internal/output"
	"github.com/DeathHapyness/spotGO/internal/song"
)

type Finder interface {
	Find(ctx context.Context, s song.Song) (matching.Scored, error)
}

type Fetcher interface {
	// Fetch baixa o áudio de url para dentro de dir e diz o arquivo e o codec.
	Fetch(ctx context.Context, url string, f ffmpeg.Format, dir string) (file, codec string, err error)
}

type Converter interface {
	Convert(ctx context.Context, job ffmpeg.Job) error
}

type Covers interface {
	Cover(ctx context.Context, url string) ([]byte, error)
}

type Lyrics interface {
	Get(ctx context.Context, s song.Song) (lyrics.Lyrics, error)
}

type Item struct {
	Song         song.Song
	ListName     string
	ListPosition int
	ListLength   int
}

type Status int

const (
	Pending Status = iota
	Downloaded
	Skipped
	NotFound
	Failed
	Canceled
	// Archived: pulada pelo Options.Skip (o --archive), sem olhar o disco.
	Archived
)

type Outcome struct {
	Song   song.Song
	Status Status
	Path   string
	URL    string
	Err    error
}

type Options struct {
	Dir       string
	Template  *output.Template
	Format    ffmpeg.Format
	Bitrate   string
	Overwrite bool
	// Zero usa o padrão de cada estágio.
	SearchWorkers   int
	DownloadWorkers int
	ConvertWorkers  int
	// Skip pula a faixa antes de tudo (nil = nenhuma).
	Skip func(Item) bool
	// ErrNotFound é o erro do Finder que significa "não achou" (e não falha de rede).
	ErrNotFound error
}

// RelPath é o caminho do arquivo final de it, relativo a Dir.
func (o Options) RelPath(it Item) string {
	return o.Template.Path(output.Vars{
		Song: it.Song, Ext: string(o.Format),
		ListName: it.ListName, ListPosition: it.ListPosition, ListLength: it.ListLength,
	})
}

type Pipeline struct {
	Finder    Finder
	Fetcher   Fetcher
	Converter Converter
	Covers    Covers
	// Lyrics nil = sem letra. LyricsTag grava na tag; LyricsFile grava o .lrc
	// ao lado (só quando há letra sincronizada).
	Lyrics     Lyrics
	LyricsTag  bool
	LyricsFile bool
	Options
	// OnDone é chamado (um de cada vez) quando uma faixa termina.
	OnDone func(i int, o Outcome)
	// OnStage é chamado quando uma faixa entra num estágio ("buscando", "baixando", "convertendo").
	OnStage func(i int, stage string)
}

type job struct {
	i       int
	path    string
	url     string
	file    string
	codec   string
	workDir string
}

func or(n, def int) int {
	if n > 0 {
		return n
	}
	return def
}

// Run processa as faixas e devolve um Outcome por item, na ordem de entrada.
func (p *Pipeline) Run(ctx context.Context, items []Item) []Outcome {
	out := make([]Outcome, len(items))
	for i, it := range items {
		out[i].Song = it.Song
	}
	var mu sync.Mutex
	finish := func(j job, o Outcome) {
		if j.workDir != "" {
			os.RemoveAll(j.workDir)
		}
		mu.Lock()
		defer mu.Unlock()
		o.Song = items[j.i].Song
		if o.Path == "" {
			o.Path = j.path
		}
		if o.URL == "" {
			o.URL = j.url
		}
		out[j.i] = o
		if p.OnDone != nil {
			p.OnDone(j.i, o)
		}
	}
	stage := func(i int, s string) {
		if p.OnStage != nil {
			mu.Lock()
			p.OnStage(i, s)
			mu.Unlock()
		}
	}
	fail := func(j job, err error) {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		st := Failed
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			st = Canceled
		}
		finish(j, Outcome{Status: st, Err: err})
	}

	searchCh := make(chan job)
	fetchCh := make(chan job)
	convCh := make(chan job)

	send := func(ch chan<- job, j job) bool {
		select {
		case ch <- j:
			return true
		case <-ctx.Done():
			fail(j, ctx.Err())
			return false
		}
	}

	go func() {
		defer close(searchCh)
		seen := map[string]bool{}
		for i, it := range items {
			if ctx.Err() != nil {
				fail(job{i: i}, ctx.Err())
				continue
			}
			j := job{i: i, path: filepath.Join(p.Dir, p.RelPath(it))}
			if p.Skip != nil && p.Skip(it) {
				finish(j, Outcome{Status: Archived})
				continue
			}
			if seen[j.path] {
				finish(j, Outcome{Status: Skipped})
				continue
			}
			seen[j.path] = true
			if !p.Overwrite {
				if _, err := os.Stat(j.path); err == nil {
					finish(j, Outcome{Status: Skipped})
					continue
				}
			}
			send(searchCh, j)
		}
	}()

	run := func(n int, in <-chan job, next chan<- job, work func(j job) (job, bool)) {
		var wg sync.WaitGroup
		for range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := range in {
					if ctx.Err() != nil {
						fail(j, ctx.Err())
						continue
					}
					if j, ok := work(j); ok && next != nil {
						send(next, j)
					}
				}
			}()
		}
		go func() {
			wg.Wait()
			if next != nil {
				close(next)
			}
		}()
	}

	run(or(p.SearchWorkers, 4), searchCh, fetchCh, func(j job) (job, bool) {
		stage(j.i, "buscando")
		found, err := p.Finder.Find(ctx, items[j.i].Song)
		if err != nil {
			if p.ErrNotFound != nil && errors.Is(err, p.ErrNotFound) {
				finish(j, Outcome{Status: NotFound, Err: err})
			} else {
				fail(j, err)
			}
			return j, false
		}
		j.url = found.Result.URL
		return j, true
	})

	run(or(p.DownloadWorkers, 4), fetchCh, convCh, func(j job) (job, bool) {
		stage(j.i, "baixando")
		dir, err := os.MkdirTemp("", "spotgo-dl-*")
		if err != nil {
			fail(j, err)
			return j, false
		}
		j.workDir = dir
		j.file, j.codec, err = p.Fetcher.Fetch(ctx, j.url, p.Format, dir)
		if err != nil {
			fail(j, err)
			return j, false
		}
		return j, true
	})

	var wg sync.WaitGroup
	for range or(p.ConvertWorkers, runtime.NumCPU()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range convCh {
				if ctx.Err() != nil {
					fail(j, ctx.Err())
					continue
				}
				p.convert(ctx, j, items[j.i].Song, stage, finish, fail)
			}
		}()
	}
	wg.Wait()

	for i := range out {
		if out[i].Status == Pending {
			out[i].Status, out[i].Err = Canceled, ctx.Err()
		}
	}
	return out
}

func (p *Pipeline) convert(ctx context.Context, j job, s song.Song, stage func(int, string),
	finish func(job, Outcome), fail func(job, error)) {
	stage(j.i, "convertendo")
	var cover []byte
	if p.Covers != nil && s.CoverURL != "" {
		// Sem capa a faixa ainda vale a pena; o erro da capa não derruba a faixa.
		cover, _ = p.Covers.Cover(ctx, s.CoverURL)
	}
	var lyr lyrics.Lyrics
	if p.Lyrics != nil && (p.LyricsTag || p.LyricsFile) {
		// Como a capa: sem letra a faixa ainda vale a pena.
		lyr, _ = p.Lyrics.Get(ctx, s)
	}
	job := ffmpeg.Job{
		Input: j.file, InputCodec: j.codec, Output: j.path,
		Format: p.Format, Bitrate: p.Bitrate, Song: s, Cover: cover,
	}
	if p.LyricsTag {
		job.Lyrics = lyr.Text()
	}
	if err := p.Converter.Convert(ctx, job); err != nil {
		fail(j, err)
		return
	}
	if p.LyricsFile && lyr.Synced != "" {
		if err := writeLRC(j.path, lyr.Synced); err != nil {
			fail(j, fmt.Errorf("gravando o .lrc: %w", err))
			return
		}
	}
	finish(j, Outcome{Status: Downloaded})
}

// LRCPath é o .lrc que acompanha o arquivo de áudio.
func LRCPath(audio string) string {
	return strings.TrimSuffix(audio, filepath.Ext(audio)) + ".lrc"
}

func writeLRC(audio, text string) error {
	path := LRCPath(audio)
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.WriteString(text + "\n")
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), 0o644)
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
