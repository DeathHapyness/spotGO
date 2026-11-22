package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/DeathHapyness/spotGO/internal/song"
	"github.com/DeathHapyness/spotGO/internal/source"
)

var kindNames = map[song.Kind]string{
	song.KindTrack:    "faixa",
	song.KindAlbum:    "álbum",
	song.KindPlaylist: "playlist",
	song.KindArtist:   "artista",
}

func newSaveCmd(newResolver func(progress func(done, total int)) resolver) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "save <link>...",
		Short: "Mostra os metadados das faixas sem baixar nada",
		Long: `Lê os links do Spotify (faixa, álbum, playlist ou artista) e mostra os
metadados que o spotgo usaria no download: título, artistas, álbum, duração e ISRC.
Não precisa de conta nem de credenciais.`,
		Example: `  spotgo save https://open.spotify.com/album/0ETFjACtuP2ADo6LFhL6HN
  spotgo save --json https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M | jq`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			stderr := cmd.ErrOrStderr()
			progress := progressPrinter(stderr)
			r := newResolver(progress)

			var results []source.Result
			failed := 0
			for i, link := range args {
				res, err := resolveLink(cmd, r, link)
				clearProgress(stderr, progress)
				if err != nil {
					failed++
					fmt.Fprintf(stderr, "erro: %s: %v\n", link, err)
					continue
				}
				if asJSON {
					results = append(results, res)
				} else {
					if i > 0 {
						fmt.Fprintln(cmd.OutOrStdout())
					}
					printResult(cmd.OutOrStdout(), res)
				}
				printWarnings(stderr, res)
			}
			if asJSON {
				if err := writeJSON(cmd.OutOrStdout(), results); err != nil {
					return err
				}
			}
			if failed > 0 {
				return fmt.Errorf("%d de %d links falharam", failed, len(args))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "saída em JSON (um objeto por link)")
	return cmd
}

func resolveLink(cmd *cobra.Command, r resolver, link string) (source.Result, error) {
	ref, err := song.ParseURL(link)
	if err != nil {
		return source.Result{}, err
	}
	return r.Resolve(cmd.Context(), ref)
}

func printResult(w io.Writer, r source.Result) {
	fmt.Fprintf(w, "%s — %s  [%s, %d faixas]\n", r.Name, r.Artist, kindNames[r.Ref.Kind], len(r.Songs))
	for i, s := range r.Songs {
		fmt.Fprintf(w, "%3d. %s — %s | %s | %s | %s\n", i+1, s.Title, strings.Join(s.Artists, ", "),
			orDash(s.Album), formatDuration(s.Duration), orDash(s.ISRC))
	}
}

func printWarnings(w io.Writer, r source.Result) {
	if r.Unmatched > 0 {
		fmt.Fprintf(w, "aviso: %d de %d faixas ficaram sem dados do Deezer (sem número da faixa, ISRC ou capa em alta)\n", r.Unmatched, len(r.Songs))
	}
	if r.Failed > 0 {
		fmt.Fprintf(w, "aviso: %d consultas ao Deezer falharam por erro de rede; rode de novo para tentar outra vez\n", r.Failed)
	}
	if r.Truncated {
		fmt.Fprintln(w, "aviso: o Spotify só mostra 100 faixas desta playlist pelo embed, e ela pode ter mais. "+
			"Com Premium: copie-a para uma playlist sua e use o login (em breve). "+
			"Sem Premium: divida-a em playlists de até 100 faixas.")
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func formatDuration(d time.Duration) string {
	secs := int(d / time.Second)
	return fmt.Sprintf("%d:%02d", secs/60, secs%60)
}

// progressPrinter devolve nil quando o stderr não é um terminal (pipe, arquivo, testes).
func progressPrinter(w io.Writer) func(done, total int) {
	f, ok := w.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return nil
	}
	return func(done, total int) {
		fmt.Fprintf(w, "\rconsultando o Deezer: %d/%d", done, total)
	}
}

func clearProgress(w io.Writer, progress func(int, int)) {
	if progress != nil {
		fmt.Fprint(w, "\r\033[K")
	}
}

type jsonResult struct {
	Kind      song.Kind  `json:"kind"`
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Artist    string     `json:"artist,omitempty"`
	Truncated bool       `json:"truncated,omitempty"`
	Songs     []jsonSong `json:"songs"`
}

type jsonSong struct {
	Title       string   `json:"title"`
	Artists     []string `json:"artists"`
	Album       string   `json:"album,omitempty"`
	AlbumArtist string   `json:"album_artist,omitempty"`
	TrackNumber int      `json:"track_number,omitempty"`
	TrackCount  int      `json:"track_count,omitempty"`
	DiscNumber  int      `json:"disc_number,omitempty"`
	DurationMS  int64    `json:"duration_ms"`
	ReleaseDate string   `json:"release_date,omitempty"`
	ISRC        string   `json:"isrc,omitempty"`
	CoverURL    string   `json:"cover_url,omitempty"`
	Explicit    bool     `json:"explicit"`
	SpotifyID   string   `json:"spotify_id,omitempty"`
}

func writeJSON(w io.Writer, results []source.Result) error {
	out := make([]jsonResult, 0, len(results))
	for _, r := range results {
		jr := jsonResult{Kind: r.Ref.Kind, ID: r.Ref.ID, Name: r.Name, Artist: r.Artist, Truncated: r.Truncated, Songs: []jsonSong{}}
		for _, s := range r.Songs {
			jr.Songs = append(jr.Songs, jsonSong{
				Title: s.Title, Artists: s.Artists, Album: s.Album, AlbumArtist: s.AlbumArtist,
				TrackNumber: s.TrackNumber, TrackCount: s.TrackCount, DiscNumber: s.DiscNumber,
				DurationMS: s.Duration.Milliseconds(), ReleaseDate: s.ReleaseDate, ISRC: s.ISRC,
				CoverURL: s.CoverURL, Explicit: s.Explicit, SpotifyID: s.SpotifyID,
			})
		}
		out = append(out, jr)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
