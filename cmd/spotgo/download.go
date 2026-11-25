package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/DeathHapyness/spotGO/internal/deps"
	"github.com/DeathHapyness/spotGO/internal/download"
	"github.com/DeathHapyness/spotGO/internal/ffmpeg"
	"github.com/DeathHapyness/spotGO/internal/output"
	"github.com/DeathHapyness/spotGO/internal/provider"
	"github.com/DeathHapyness/spotGO/internal/song"
	"github.com/DeathHapyness/spotGO/internal/source"
	"github.com/DeathHapyness/spotGO/internal/syncstate"
)

type downloadFlags struct {
	output    string
	format    string
	bitrate   string
	overwrite bool
	threads   int
	yes       bool
	tools     map[deps.Tool]*string
	audio     string
	lyrics    string
	m3u       bool
}

// downloadEnv é o que o comando precisa do mundo de fora (trocado nos testes).
type downloadEnv struct {
	resolver resolver
	finder   download.Finder
	lyrics   download.Lyrics
	// newFinder monta a cadeia do --audio; nil usa finder.
	newFinder func(audio []string, ytdlp string) (download.Finder, error)
	covers    download.Covers
	// tools localiza (e, se preciso, instala) os binários e monta o yt-dlp e o ffmpeg.
	tools func(ctx context.Context, f downloadFlags, confirm func(missing []deps.Tool) error) (download.Fetcher, download.Converter, []string, error)
}

func newDownloadCmd(newEnv func(progress func(done, total int)) downloadEnv) *cobra.Command {
	f := downloadFlags{tools: map[deps.Tool]*string{}}
	var archivePath string
	cmd := &cobra.Command{
		Use:   "download <link>...",
		Short: "Baixa as faixas com capa e metadados",
		Long: `Lê os links do Spotify (faixa, álbum, playlist ou artista), procura cada faixa
no YouTube Music, baixa o áudio com o yt-dlp e grava o arquivo final com o
ffmpeg, já com as tags e a capa. Faixas que já existem são puladas.

` + templateHelp,
		Example: `  spotgo download https://open.spotify.com/track/2EqlS6tkEnglzr7tkKAAYD
  spotgo download -o "{album-artist}/{album}/{track-number} {title}" --format opus <álbum>
  spotgo download -o "{list-name}/{list-position} - {artists} - {title}" <playlist>
  spotgo download --archive baixadas.txt <playlist>`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var archive *syncstate.Archive
			if archivePath != "" {
				a, err := syncstate.OpenArchive(archivePath)
				if err != nil {
					return fmt.Errorf("--archive: %w", err)
				}
				archive = a
			}
			s, err := newSession(cmd, newEnv, f, "")
			if err != nil {
				return err
			}
			items, groups, linkErrs, err := s.resolve(args)
			if err != nil {
				return err
			}

			if archive != nil {
				s.p.Skip = func(it download.Item) bool { return archive.Has(it.Song.SpotifyID) }
			}
			outs, counts := s.run(items, func(o download.Outcome) {
				if archive != nil && (o.Status == download.Downloaded || o.Status == download.Skipped) {
					if err := archive.Add(o.Song.SpotifyID); err != nil {
						fmt.Fprintln(cmd.ErrOrStderr(), "aviso: --archive:", err)
					}
				}
			})
			if f.m3u {
				s.writeM3U(".", groups, outs)
			}
			return s.finish(counts, linkErrs, len(args))
		},
	}
	addDownloadFlags(cmd, &f)
	cmd.Flags().StringVar(&archivePath, "archive", "", "arquivo com as faixas já baixadas; elas são puladas, e as novas entram nele")
	return cmd
}

const templateHelp = `Variáveis do --output: {title} {artists} {artist} {album} {album-artist}
{track-number} {tracks-count} {disc-number} {year} {date} {isrc} {track-id}
{duration} {list-name} {list-position} {list-length} {output-ext}.
Uma / no template cria pastas.`

func addDownloadFlags(cmd *cobra.Command, f *downloadFlags) {
	fl := cmd.Flags()
	fl.StringVarP(&f.output, "output", "o", output.DefaultTemplate, "template do caminho de cada arquivo")
	fl.StringVar(&f.format, "format", "mp3", "formato: mp3, m4a, opus, ogg, flac ou wav")
	fl.StringVar(&f.bitrate, "bitrate", "", `bitrate do áudio (ex.: "320k"); o padrão evita reencodar à toa`)
	fl.BoolVar(&f.overwrite, "overwrite", false, "baixa de novo faixas que já existem")
	fl.IntVar(&f.threads, "threads", 4, "buscas e downloads simultâneos")
	fl.BoolVarP(&f.yes, "yes", "y", false, "baixa o yt-dlp, o ffmpeg ou o deno sem perguntar, se faltarem")
	fl.StringVar(&f.lyrics, "lyrics", "tag", "letras do LRCLIB: tag (na tag do arquivo), lrc (arquivo .lrc ao lado), tag,lrc ou none")
	fl.BoolVar(&f.m3u, "m3u", false, "grava uma playlist .m3u8 para cada álbum, playlist ou artista")
	fl.StringVar(&f.audio, "audio", defaultAudio, "provedores de áudio, em ordem: "+strings.Join(audioProviders, ", "))
	for _, t := range deps.Tools {
		f.tools[t] = fl.String(string(t), "", "caminho do "+string(t))
	}
}

const defaultAudio = "youtube-music,youtube"

var audioProviders = []string{"youtube-music", "youtube", "soundcloud", "bandcamp"}

func parseLyrics(s string) (tag, file bool, err error) {
	for _, v := range strings.Split(strings.ToLower(s), ",") {
		switch strings.TrimSpace(v) {
		case "tag":
			tag = true
		case "lrc":
			file = true
		case "none", "":
		default:
			return false, false, fmt.Errorf("--lyrics: valor desconhecido %q (use tag, lrc, tag,lrc ou none)", v)
		}
	}
	return tag, file, nil
}

func parseAudio(s string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, n := range strings.Split(s, ",") {
		n = strings.ToLower(strings.TrimSpace(n))
		if n == "" || seen[n] {
			continue
		}
		if !slices.Contains(audioProviders, n) {
			return nil, fmt.Errorf("--audio: provedor desconhecido %q (use %s)", n, strings.Join(audioProviders, ", "))
		}
		seen[n] = true
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, errors.New("--audio: nenhum provedor")
	}
	return out, nil
}

// session junta o que o download e o sync fazem igual: validar as opções,
// achar os binários, resolver os links e rodar o pipeline mostrando o progresso.
type session struct {
	cmd      *cobra.Command
	env      downloadEnv
	progress func(done, total int)
	p        *download.Pipeline
}

func newSession(cmd *cobra.Command, newEnv func(func(int, int)) downloadEnv, f downloadFlags, dir string) (*session, error) {
	format, err := ffmpeg.ParseFormat(f.format)
	if err != nil {
		return nil, err
	}
	tmpl, err := output.Parse(f.output)
	if err != nil {
		return nil, err
	}
	if f.threads < 1 {
		return nil, errors.New("--threads precisa ser pelo menos 1")
	}
	audio, err := parseAudio(f.audio)
	if err != nil {
		return nil, err
	}
	lyrTag, lyrFile, err := parseLyrics(f.lyrics)
	if err != nil {
		return nil, err
	}
	s := &session{cmd: cmd, progress: progressPrinter(cmd.ErrOrStderr())}
	s.env = newEnv(s.progress)
	fetcher, conv, warnings, err := s.env.tools(cmd.Context(), f, confirmInstall(cmd, f.yes))
	if err != nil {
		return nil, err
	}
	for _, w := range warnings {
		fmt.Fprintln(cmd.ErrOrStderr(), "aviso:", w)
	}
	finder := s.env.finder
	if s.env.newFinder != nil {
		ytdlp := ""
		if y, ok := fetcher.(*download.YTDLP); ok {
			ytdlp = y.Path
		}
		if finder, err = s.env.newFinder(audio, ytdlp); err != nil {
			return nil, err
		}
	}
	s.p = &download.Pipeline{
		Finder: finder, Fetcher: fetcher, Converter: conv, Covers: s.env.covers,
		Lyrics: s.env.lyrics, LyricsTag: lyrTag, LyricsFile: lyrFile,
		Options: download.Options{
			Dir: dir, Template: tmpl, Format: format, Bitrate: f.bitrate, Overwrite: f.overwrite,
			SearchWorkers: f.threads, DownloadWorkers: f.threads,
			ErrNotFound: provider.ErrNotFound,
		},
	}
	return s, nil
}

var errInterrupted = errors.New("interrompido")

// group é a faixa de itens que veio de um link (para o .m3u8).
type group struct {
	name       string
	start, end int
}

// resolve só devolve erro no Ctrl+C; links com erro são contados e mostrados.
func (s *session) resolve(links []string) ([]download.Item, []group, int, error) {
	stderr := s.cmd.ErrOrStderr()
	var items []download.Item
	var groups []group
	linkErrs := 0
	for _, link := range links {
		res, err := resolveLink(s.cmd, s.env.resolver, link)
		clearProgress(stderr, s.progress)
		if s.cmd.Context().Err() != nil {
			return nil, nil, 0, errInterrupted
		}
		if err != nil {
			linkErrs++
			fmt.Fprintf(stderr, "erro: %s: %v\n", link, err)
			continue
		}
		printWarnings(stderr, res)
		start := len(items)
		items = append(items, toItems(res)...)
		if res.Ref.Kind != song.KindTrack && res.Name != "" {
			groups = append(groups, group{res.Name, start, len(items)})
		}
	}
	return items, groups, linkErrs, nil
}

// writeM3U grava <dir>/<nome da lista>.m3u8 com as faixas que existem em disco.
// As puladas pelo --archive não entram: o arquivo pode nem estar mais aqui.
func (s *session) writeM3U(dir string, groups []group, outs []download.Outcome) {
	for _, g := range groups {
		var entries []output.M3UEntry
		for i := g.start; i < g.end && i < len(outs); i++ {
			o := outs[i]
			if o.Status != download.Downloaded && o.Status != download.Skipped {
				continue
			}
			entries = append(entries, output.M3UEntry{Path: o.Path, Title: songLabel(o.Song), Duration: o.Song.Duration})
		}
		path := filepath.Join(dir, output.Sanitize(g.name)+".m3u8")
		if err := output.WriteM3U(path, entries); err != nil {
			fmt.Fprintf(s.cmd.ErrOrStderr(), "aviso: %s: %v\n", path, err)
			continue
		}
		fmt.Fprintf(s.cmd.ErrOrStderr(), "playlist    %s (%d faixas)\n", s.rel(path), len(entries))
	}
}

func (s *session) run(items []download.Item, onDone func(download.Outcome)) ([]download.Outcome, map[download.Status]int) {
	stdout, stderr := s.cmd.OutOrStdout(), s.cmd.ErrOrStderr()
	counts := map[download.Status]int{}
	live := liveStatus(stderr, len(items))
	s.p.OnStage = func(i int, stage string) { live.stage(items[i].Song, stage) }
	s.p.OnDone = func(i int, o download.Outcome) {
		counts[o.Status]++
		live.clear()
		printOutcome(stdout, stderr, s.rel(o.Path), o)
		live.done()
		if onDone != nil {
			onDone(o)
		}
	}
	outs := s.p.Run(s.cmd.Context(), items)
	live.clear()
	if len(items) > 0 && s.cmd.Context().Err() == nil {
		fmt.Fprintln(stderr, summary(counts))
	}
	return outs, counts
}

func (s *session) rel(path string) string {
	if s.p.Dir == "" {
		return path
	}
	if r, err := filepath.Rel(s.p.Dir, path); err == nil {
		return r
	}
	return path
}

func (s *session) finish(counts map[download.Status]int, linkErrs, links int) error {
	switch {
	case s.cmd.Context().Err() != nil:
		return errInterrupted
	case linkErrs > 0:
		return fmt.Errorf("%d de %d links falharam", linkErrs, links)
	case counts[download.Failed] > 0:
		return fmt.Errorf("%d faixas falharam; rode de novo para tentar outra vez", counts[download.Failed])
	}
	return nil
}

func toItems(r source.Result) []download.Item {
	list := ""
	if r.Ref.Kind != song.KindTrack {
		list = r.Name
	}
	items := make([]download.Item, len(r.Songs))
	for i, s := range r.Songs {
		items[i] = download.Item{Song: s, ListName: list, ListPosition: i + 1, ListLength: len(r.Songs)}
	}
	return items
}

func printOutcome(stdout, stderr io.Writer, path string, o download.Outcome) {
	label := songLabel(o.Song)
	switch o.Status {
	case download.Downloaded:
		fmt.Fprintf(stdout, "baixado     %s  →  %s\n", label, path)
	case download.Skipped:
		fmt.Fprintf(stdout, "já existe   %s  (%s)\n", label, path)
	case download.NotFound:
		fmt.Fprintf(stdout, "não achada  %s\n", label)
	case download.Failed:
		fmt.Fprintf(stderr, "erro        %s: %v\n", label, o.Err)
	}
}

func summary(c map[download.Status]int) string {
	parts := []string{plural(c[download.Downloaded], "baixada", "baixadas")}
	if n := c[download.Skipped]; n > 0 {
		parts = append(parts, plural(n, "já existia", "já existiam"))
	}
	if n := c[download.Archived]; n > 0 {
		parts = append(parts, plural(n, "pulada pelo --archive", "puladas pelo --archive"))
	}
	if n := c[download.NotFound]; n > 0 {
		parts = append(parts, plural(n, "não achada", "não achadas"))
	}
	if n := c[download.Failed]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d com erro", n))
	}
	return strings.Join(parts, ", ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

type live struct {
	w     io.Writer
	total int
	n     int
	on    bool
}

// liveStatus mostra "[3/20] baixando Artista - Título" numa linha só, se o stderr for um terminal.
func liveStatus(w io.Writer, total int) *live {
	l := &live{w: w, total: total}
	if f, ok := w.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		l.on = true
	}
	return l
}

func (l *live) stage(s song.Song, stage string) {
	if l.on {
		fmt.Fprintf(l.w, "\r\033[K[%d/%d] %s %s", l.n, l.total, stage, songLabel(s))
	}
}

func (l *live) done() { l.n++ }

func (l *live) clear() {
	if l.on {
		fmt.Fprint(l.w, "\r\033[K")
	}
}

// confirmInstall decide se pode baixar os binários que faltam.
func confirmInstall(cmd *cobra.Command, yes bool) func(missing []deps.Tool) error {
	return func(missing []deps.Tool) error {
		names := make([]string, len(missing))
		for i, t := range missing {
			names[i] = string(t)
		}
		list := strings.Join(names, ", ")
		if yes {
			return nil
		}
		in, ok := stdinTerminal(cmd)
		if !ok {
			return fmt.Errorf("faltam %s; rode spotgo setup (ou use --yes para baixar agora)", list)
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "Faltam %s. Baixar agora as versões oficiais? [S/n] ", list)
		ans, _ := bufio.NewReader(in).ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(ans)) {
		case "", "s", "sim", "y", "yes":
			return nil
		}
		return fmt.Errorf("faltam %s; rode spotgo setup quando quiser", list)
	}
}

// ensureTools acha os três binários e instala os que faltam (depois de confirm).
func ensureTools(ctx context.Context, loc *deps.Locator, install func(context.Context, deps.Tool) (string, error),
	confirm func([]deps.Tool) error, now time.Time) (map[deps.Tool]deps.Found, []string, error) {
	found := map[deps.Tool]deps.Found{}
	var missing []deps.Tool
	for _, t := range deps.Tools {
		f, err := loc.Locate(ctx, t)
		switch {
		case errors.Is(err, deps.ErrMissing):
			missing = append(missing, t)
		case err != nil:
			return nil, nil, err
		default:
			found[t] = f
		}
	}
	if len(missing) > 0 {
		if err := confirm(missing); err != nil {
			return nil, nil, err
		}
		for _, t := range missing {
			path, err := install(ctx, t)
			if err != nil {
				return nil, nil, fmt.Errorf("instalando o %s: %w", t, err)
			}
			found[t] = deps.Found{Tool: t, Path: path, Origin: deps.FromManaged, Version: loc.Version(ctx, t, path)}
		}
	}
	var warnings []string
	if y := found[deps.YTDLP]; y.Version != "" {
		if age, ok := deps.YTDLPAge(y.Version, now); ok && age > deps.MaxYTDLPAge {
			warnings = append(warnings, fmt.Sprintf("o yt-dlp (%s) tem %d dias e pode não funcionar com o YouTube; rode spotgo setup --update",
				y.Path, int(age.Hours()/24)))
		}
	}
	return found, warnings, nil
}
