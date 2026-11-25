package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/DeathHapyness/spotGO/internal/download"
	"github.com/DeathHapyness/spotGO/internal/ffmpeg"
	"github.com/DeathHapyness/spotGO/internal/lyrics"
	"github.com/DeathHapyness/spotGO/internal/song"
)

const metaWorkers = 4

type tagger interface {
	Probe(ctx context.Context, path string, f ffmpeg.Format) (ffmpeg.Info, error)
	Convert(ctx context.Context, job ffmpeg.Job) error
}

type metaEnv struct {
	resolver resolver
	enrich   func(ctx context.Context, s song.Song) (song.Song, bool, error)
	covers   download.Covers
	lyrics   download.Lyrics
	tagger   func(ffmpegPath string) (tagger, error)
}

type metaResult struct {
	path   string
	song   song.Song
	status download.Status
	err    error
}

func newMetaCmd(newEnv func(progress func(done, total int)) metaEnv) *cobra.Command {
	var lyricsFlag, ffmpegPath string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "meta <arquivo ou pasta>...",
		Short: "Atualiza as tags, a capa e a letra de músicas que você já tem",
		Long: `Lê as tags de cada arquivo (mp3, m4a, opus, ogg, flac, wav), acha a faixa e
regrava título, artistas, álbum, número da faixa, data, ISRC, capa e letra.
O áudio é copiado sem reencodar, e as tags que o spotgo não conhece (gênero,
por exemplo) continuam lá.

Como a faixa é achada:
  1. arquivos baixados pelo spotgo têm o link do Spotify no comentário;
  2. senão, pelo título, artista e duração das tags no Deezer;
  3. sem título nas tags, pelo nome do arquivo ("Artista - Título.mp3").

Pastas são lidas por inteiro, com as subpastas.`,
		Example: `  spotgo meta "Come Together.mp3"
  spotgo meta --dry-run ~/Música
  spotgo meta --lyrics tag,lrc ~/Música/Beatles`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stdout, stderr := cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr()
			lyrTag, lyrFile, err := parseLyrics(lyricsFlag)
			if err != nil {
				return err
			}
			files, err := audioFiles(args)
			if err != nil {
				return err
			}
			if len(files) == 0 {
				return errors.New("nenhum arquivo de áudio encontrado")
			}
			env := newEnv(nil)
			tg, err := env.tagger(ffmpegPath)
			if err != nil {
				return err
			}
			w := &metaWorker{env: env, tagger: tg, lyrTag: lyrTag, lyrFile: lyrFile, dryRun: dryRun}

			counts := map[download.Status]int{}
			var mu sync.Mutex
			live := liveStatus(stderr, len(files))
			forEach(ctx, files, metaWorkers, func(path string) {
				r := w.do(ctx, path)
				mu.Lock()
				defer mu.Unlock()
				counts[r.status]++
				live.clear()
				printMeta(stdout, stderr, r, dryRun)
				live.done()
			})
			live.clear()
			if ctx.Err() != nil {
				return errInterrupted
			}
			verb := plural(counts[download.Downloaded], "atualizado", "atualizados")
			if dryRun {
				verb = plural(counts[download.Downloaded], "seria atualizado", "seriam atualizados")
			}
			parts := []string{verb}
			if n := counts[download.NotFound]; n > 0 {
				parts = append(parts, plural(n, "não achado", "não achados"))
			}
			if n := counts[download.Failed]; n > 0 {
				parts = append(parts, fmt.Sprintf("%d com erro", n))
			}
			fmt.Fprintln(stderr, strings.Join(parts, ", "))
			if n := counts[download.Failed]; n > 0 {
				return fmt.Errorf("%s", plural(n, "arquivo falhou", "arquivos falharam"))
			}
			return nil
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&lyricsFlag, "lyrics", "tag", "letras do LRCLIB: tag, lrc, tag,lrc ou none (none mantém a letra que já existe)")
	fl.BoolVar(&dryRun, "dry-run", false, "só mostra o que seria gravado, sem mudar nada")
	fl.StringVar(&ffmpegPath, "ffmpeg", "", "caminho do ffmpeg")
	return cmd
}

type metaWorker struct {
	env             metaEnv
	tagger          tagger
	lyrTag, lyrFile bool
	dryRun          bool
}

func (w *metaWorker) do(ctx context.Context, path string) metaResult {
	r := metaResult{path: path, status: download.Failed}
	f, err := ffmpeg.ParseFormat(strings.TrimPrefix(filepath.Ext(path), "."))
	if err != nil {
		r.err = err
		return r
	}
	info, err := w.tagger.Probe(ctx, path, f)
	if err != nil {
		r.err = err
		return r
	}
	s, ok, err := w.identify(ctx, songFromTags(info, path))
	r.song = s
	switch {
	case err != nil:
		r.err = err
		return r
	case !ok:
		r.status = download.NotFound
		return r
	}
	if w.dryRun {
		r.status = download.Downloaded
		return r
	}

	var cover []byte
	if w.env.covers != nil && s.CoverURL != "" {
		cover, _ = w.env.covers.Cover(ctx, s.CoverURL)
	}
	job := ffmpeg.Job{
		Input: path, Output: path, Format: f, Copy: true,
		Song: s, Cover: cover, KeepCover: info.HasCover, Extra: info.Tags,
		Lyrics: info.Tags["lyrics"],
	}
	var lyr lyrics.Lyrics
	if w.env.lyrics != nil && (w.lyrTag || w.lyrFile) {
		lyr, _ = w.env.lyrics.Get(ctx, s)
	}
	if w.lyrTag && lyr.Text() != "" {
		job.Lyrics = lyr.Text()
	}
	if err := w.tagger.Convert(ctx, job); err != nil {
		r.err = err
		return r
	}
	if w.lyrFile && lyr.Synced != "" {
		if err := os.WriteFile(download.LRCPath(path), []byte(lyr.Synced+"\n"), 0o644); err != nil {
			r.err = fmt.Errorf("gravando o .lrc: %w", err)
			return r
		}
	}
	r.status = download.Downloaded
	return r
}

// identify: pelo link do Spotify do comentário, se houver; senão pelo Deezer.
func (w *metaWorker) identify(ctx context.Context, s song.Song) (song.Song, bool, error) {
	if s.SpotifyID != "" && w.env.resolver != nil {
		res, err := w.env.resolver.Resolve(ctx, song.Ref{Kind: song.KindTrack, ID: s.SpotifyID})
		if err == nil && len(res.Songs) > 0 {
			return res.Songs[0], true, nil
		}
		if ctx.Err() != nil {
			return s, false, ctx.Err()
		}
	}
	if s.Title == "" || len(s.Artists) == 0 {
		return s, false, nil
	}
	found, ok, err := w.env.enrich(ctx, s)
	if err != nil {
		return s, false, err
	}
	return found, ok, nil
}

var spotifyTrackRe = regexp.MustCompile(`open\.spotify\.com/(?:intl-[a-z-]+/)?track/([A-Za-z0-9]{22})`)

// songFromTags monta o que dá para saber do arquivo, para achar a faixa.
func songFromTags(info ffmpeg.Info, path string) song.Song {
	t := info.Tags
	s := song.Song{
		Title:    strings.TrimSpace(t["title"]),
		Album:    strings.TrimSpace(t["album"]),
		ISRC:     strings.TrimSpace(t["isrc"]),
		Duration: info.Duration,
	}
	if a := strings.TrimSpace(t["artist"]); a != "" {
		s.Artists = splitTagArtists(a)
	}
	for _, k := range []string{"comment", "url", "website"} {
		if m := spotifyTrackRe.FindStringSubmatch(t[k]); m != nil {
			s.SpotifyID = m[1]
			break
		}
	}
	if s.Title == "" {
		base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		if artist, title, ok := strings.Cut(base, " - "); ok && len(s.Artists) == 0 {
			s.Artists, s.Title = splitTagArtists(artist), strings.TrimSpace(title)
		} else {
			s.Title = strings.TrimSpace(base)
		}
	}
	return s
}

// splitTagArtists separa só por ", " e ";": a "/" aparece em nomes como AC/DC.
func splitTagArtists(s string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ';' }) {
		for _, a := range strings.Split(part, ", ") {
			if a = strings.TrimSpace(a); a != "" {
				out = append(out, a)
			}
		}
	}
	return out
}

func audioFiles(args []string) ([]string, error) {
	isAudio := func(p string) bool {
		_, err := ffmpeg.ParseFormat(strings.TrimPrefix(filepath.Ext(p), "."))
		return err == nil && !strings.HasPrefix(filepath.Base(p), ".")
	}
	var out []string
	for _, a := range args {
		st, err := os.Stat(a)
		if err != nil {
			return nil, err
		}
		if !st.IsDir() {
			if !isAudio(a) {
				return nil, fmt.Errorf("%s: não é um formato que o spotgo conhece", a)
			}
			out = append(out, a)
			continue
		}
		err = filepath.WalkDir(a, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && isAudio(p) {
				out = append(out, p)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

func forEach(ctx context.Context, items []string, n int, fn func(string)) {
	ch := make(chan string)
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := range ch {
				fn(it)
			}
		}()
	}
	for _, it := range items {
		select {
		case ch <- it:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
	}
	close(ch)
	wg.Wait()
}

func printMeta(stdout, stderr interface{ Write([]byte) (int, error) }, r metaResult, dryRun bool) {
	switch r.status {
	case download.Downloaded:
		verb := "atualizado "
		if dryRun {
			verb = "atualizaria"
		}
		fmt.Fprintf(stdout, "%s %s  →  %s | %s | %s\n", verb, r.path, songLabel(r.song), orDash(r.song.Album), orDash(r.song.ISRC))
	case download.NotFound:
		fmt.Fprintf(stdout, "não achado  %s\n", r.path)
	default:
		fmt.Fprintf(stderr, "erro        %s: %v\n", r.path, r.err)
	}
}
