package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/DeathHapyness/spotGO/internal/deps"
	"github.com/DeathHapyness/spotGO/internal/download"
	"github.com/DeathHapyness/spotGO/internal/syncstate"
)

func newSyncCmd(newEnv func(progress func(done, total int)) downloadEnv) *cobra.Command {
	f := downloadFlags{tools: map[deps.Tool]*string{}}
	var dir string
	var keep bool
	cmd := &cobra.Command{
		Use:   "sync [link]...",
		Short: "Deixa uma pasta igual a uma playlist: baixa as novas e apaga as removidas",
		Long: `Na primeira vez, recebe os links e as opções (--output, --format, --bitrate) e
guarda tudo em ` + syncstate.FileName + ` na pasta. Depois, basta rodar
"spotgo sync" de novo nessa pasta: o spotgo baixa as faixas novas, renomeia as
que mudaram de posição e apaga as que saíram da playlist. Passar links de novo
substitui os que estavam guardados.

Só são apagados arquivos que o próprio sync baixou. Se algum link falhar,
nada é apagado nessa rodada.

` + templateHelp,
		Example: `  spotgo sync -o "{list-position} - {artists} - {title}" https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M
  spotgo sync                 # de novo, na mesma pasta
  spotgo sync --dir ~/Música/Mix`,
		RunE: func(cmd *cobra.Command, args []string) error {
			stdout, stderr := cmd.OutOrStdout(), cmd.ErrOrStderr()
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			st, found, err := syncstate.Load(dir)
			if err != nil {
				return err
			}
			if len(args) > 0 {
				st.Links = args
			} else if !found {
				return fmt.Errorf("nenhum %s em %s; na primeira vez, passe os links: spotgo sync <link>", syncstate.FileName, dir)
			}
			fl := cmd.Flags()
			pick := func(name string, flag *string, saved *string) {
				if fl.Changed(name) || *saved == "" {
					*saved = *flag
				}
				*flag = *saved
			}
			pick("output", &f.output, &st.Template)
			pick("format", &f.format, &st.Format)
			pick("audio", &f.audio, &st.Audio)
			pick("lyrics", &f.lyrics, &st.Lyrics)
			if fl.Changed("m3u") {
				st.M3U = f.m3u
			}
			if fl.Changed("bitrate") {
				st.Bitrate = f.bitrate
			}
			f.bitrate = st.Bitrate

			s, err := newSession(cmd, newEnv, f, dir)
			if err != nil {
				return err
			}
			items, groups, linkErrs, err := s.resolve(st.Links)
			if err != nil {
				return err
			}

			current := map[string]string{}
			for _, it := range items {
				current[key(it)] = filepath.ToSlash(s.p.RelPath(it))
			}
			moved, err := syncstate.ApplyMoves(dir, syncstate.Moves(st.Files, current))
			for _, m := range moved {
				fmt.Fprintf(stdout, "movido      %s  →  %s\n", m.From, m.To)
			}
			if err != nil {
				return fmt.Errorf("renomeando: %w", err)
			}

			ok := map[string]bool{}
			files := map[string]string{}
			byPath := map[string]string{}
			for _, it := range items {
				byPath[filepath.Join(dir, s.p.RelPath(it))] = key(it)
			}
			outs, counts := s.run(items, func(o download.Outcome) {
				k := byPath[o.Path]
				if o.Status == download.Downloaded || o.Status == download.Skipped {
					ok[k] = true
					files[k] = current[k]
				}
			})
			// Faixa que falhou desta vez continua no estado, se o arquivo antigo existe.
			for k, p := range st.Files {
				if _, still := current[k]; still && !ok[k] {
					files[k] = p
				}
			}

			deleted := 0
			interrupted := cmd.Context().Err() != nil
			switch {
			case interrupted:
			case linkErrs > 0:
				fmt.Fprintln(stderr, "aviso: algum link falhou; nada foi apagado nesta rodada")
				keepOld(files, st.Files)
			case keep:
				keepOld(files, st.Files)
			default:
				for _, rel := range syncstate.Stale(st.Files, current, ok) {
					removed, err := syncstate.Remove(dir, rel)
					if err != nil {
						fmt.Fprintf(stderr, "aviso: não consegui apagar %s: %v\n", rel, err)
						continue
					}
					if removed {
						deleted++
						fmt.Fprintf(stdout, "apagado     %s\n", rel)
					}
				}
			}
			if interrupted {
				keepOld(files, st.Files)
			}

			if st.M3U && !interrupted {
				s.writeM3U(dir, groups, outs)
			}
			st.Files = files
			if err := syncstate.Save(dir, st); err != nil {
				return fmt.Errorf("salvando %s: %w", syncstate.FileName, err)
			}
			if deleted > 0 || len(moved) > 0 {
				fmt.Fprintf(stderr, "%s, %s\n", plural(len(moved), "movida", "movidas"), plural(deleted, "apagada", "apagadas"))
			}
			return s.finish(counts, linkErrs, len(st.Links))
		},
	}
	addDownloadFlags(cmd, &f)
	cmd.Flags().StringVar(&dir, "dir", ".", "pasta sincronizada")
	cmd.Flags().BoolVar(&keep, "keep", false, "não apaga as faixas que saíram da playlist")
	return cmd
}

// keepOld mantém no estado os arquivos antigos que não foram apagados, para
// uma rodada futura ainda poder apagá-los.
func keepOld(files, old map[string]string) {
	for k, p := range old {
		if _, has := files[k]; !has {
			files[k] = p
		}
	}
}

func key(it download.Item) string {
	if it.Song.SpotifyID != "" {
		return it.Song.SpotifyID
	}
	return "sem-id:" + it.Song.MainArtist() + " - " + it.Song.Title
}
