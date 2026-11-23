package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/DeathHapyness/spotGO/internal/matching"
	"github.com/DeathHapyness/spotGO/internal/provider/ytmusic"
	"github.com/DeathHapyness/spotGO/internal/song"
)

// Buscas simultâneas no YouTube Music; o limitador do fetch ainda espaça as requisições.
const urlWorkers = 4

type finder interface {
	Find(ctx context.Context, s song.Song) (matching.Scored, error)
}

type match struct {
	song  song.Song
	found matching.Scored
	err   error
}

type jsonURL struct {
	Title   string   `json:"title"`
	Artists []string `json:"artists"`
	URL     string   `json:"url"`
	Score   float64  `json:"score"`
}

func newURLCmd(newResolver func(progress func(done, total int)) resolver, newFinder func() finder) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "url <link>...",
		Short: "Mostra o link do YouTube Music encontrado para cada faixa",
		Long: `Lê os links do Spotify (faixa, álbum, playlist ou artista), procura cada faixa
no YouTube Music e mostra o link do áudio que o spotgo baixaria. Não baixa nada.`,
		Example: `  spotgo url https://open.spotify.com/album/0ETFjACtuP2ADo6LFhL6HN
  spotgo url --json https://open.spotify.com/track/2EqlS6tkEnglzr7tkKAAYD | jq`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stdout, stderr := cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr()
			progress := progressPrinter(stderr)
			r := newResolver(progress)
			f := newFinder()

			var all []match
			linkErrs, searchErrs := 0, 0
			for _, link := range args {
				res, err := resolveLink(cmd, r, link)
				clearProgress(stderr, progress)
				if ctx.Err() != nil {
					return fmt.Errorf("interrompido: %w", ctx.Err())
				}
				if err != nil {
					linkErrs++
					fmt.Fprintf(stderr, "erro: %s: %v\n", link, err)
					continue
				}
				printWarnings(stderr, res)

				matches := findAll(ctx, f, res.Songs, searchProgress(stderr))
				if ctx.Err() != nil {
					return fmt.Errorf("interrompido: %w", ctx.Err())
				}
				for _, m := range matches {
					if m.err != nil && !errors.Is(m.err, ytmusic.ErrNotFound) {
						searchErrs++
						fmt.Fprintf(stderr, "erro: %s: %v\n", songLabel(m.song), m.err)
						continue
					}
					if !asJSON {
						printMatch(stdout, m)
					}
				}
				all = append(all, matches...)
			}

			if asJSON {
				if err := writeURLJSON(stdout, all); err != nil {
					return err
				}
			}
			ok := 0
			for _, m := range all {
				if m.err == nil {
					ok++
				}
			}
			if len(all) > 0 {
				fmt.Fprintf(stderr, "%d de %d faixas encontradas\n", ok, len(all))
			}
			switch {
			case linkErrs > 0:
				return fmt.Errorf("%d de %d links falharam", linkErrs, len(args))
			case searchErrs > 0:
				return fmt.Errorf("%d buscas falharam; rode de novo para tentar outra vez", searchErrs)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "saída em JSON (um objeto por faixa)")
	return cmd
}

// findAll busca as faixas em paralelo e devolve os resultados na ordem de entrada.
func findAll(ctx context.Context, f finder, songs []song.Song, progress func(done, total int)) []match {
	out := make([]match, len(songs))
	sem := make(chan struct{}, urlWorkers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0
	for i, s := range songs {
		wg.Add(1)
		go func(i int, s song.Song) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				out[i] = match{song: s, err: ctx.Err()}
				return
			}
			defer func() { <-sem }()
			found, err := f.Find(ctx, s)
			out[i] = match{song: s, found: found, err: err}
			if progress != nil {
				mu.Lock()
				done++
				progress(done, len(songs))
				mu.Unlock()
			}
		}(i, s)
	}
	wg.Wait()
	if progress != nil {
		progress(-1, 0)
	}
	return out
}

func searchProgress(w io.Writer) func(done, total int) {
	f, ok := w.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return nil
	}
	return func(done, total int) {
		if done < 0 {
			fmt.Fprint(w, "\r\033[K")
			return
		}
		fmt.Fprintf(w, "\rbuscando no YouTube Music: %d/%d", done, total)
	}
}

func songLabel(s song.Song) string {
	if len(s.Artists) == 0 {
		return s.Title
	}
	return strings.Join(s.Artists, ", ") + " - " + s.Title
}

func printMatch(w io.Writer, m match) {
	if m.err != nil {
		fmt.Fprintf(w, "%s\tnão encontrada\n", songLabel(m.song))
		return
	}
	fmt.Fprintf(w, "%s\t%s\n", songLabel(m.song), m.found.Result.URL)
}

func writeURLJSON(w io.Writer, ms []match) error {
	out := make([]jsonURL, 0, len(ms))
	for _, m := range ms {
		if m.err != nil && !errors.Is(m.err, ytmusic.ErrNotFound) {
			continue
		}
		j := jsonURL{Title: m.song.Title, Artists: m.song.Artists}
		if m.err == nil {
			j.URL, j.Score = m.found.Result.URL, m.found.Score
		}
		out = append(out, j)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
