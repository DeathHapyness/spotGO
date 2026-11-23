package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/DeathHapyness/spotGO/internal/deps"
)

type setupDeps struct {
	locate  func(ctx context.Context, t deps.Tool) ([]deps.Found, error)
	install func(ctx context.Context, t deps.Tool) (string, error)
	version func(ctx context.Context, t deps.Tool, path string) string
	now     func() time.Time
}

func newSetupCmd(newDeps func(progress func(t deps.Tool, done, total int64)) setupDeps) *cobra.Command {
	var update bool
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Baixa e confere o yt-dlp, o ffmpeg e o deno",
		Long: `Baixa as versões oficiais do yt-dlp, do ffmpeg e do deno para uma pasta só do
spotgo, conferindo o SHA-256 publicado de cada uma. Ferramentas que já estão
instaladas no sistema são aproveitadas, exceto um yt-dlp com mais de 60 dias.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stdout, stderr := cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr()
			progress := downloadProgress(stderr)
			d := newDeps(progress)

			failed := 0
			var ytdlp *deps.Found
			for _, t := range deps.Tools {
				f, err := setupTool(ctx, d, t, update)
				if progress != nil {
					fmt.Fprint(stderr, "\r\033[K")
				}
				if ctx.Err() != nil || errors.Is(err, context.Canceled) {
					return errors.New("interrompido")
				}
				switch {
				case err != nil && errors.Is(err, deps.ErrUnsupported):
					fmt.Fprintf(stdout, "%-7s %-6s  %v\n", "falta", t, err)
				case err != nil:
					failed++
					fmt.Fprintf(stdout, "%-7s %-6s  %v\n", "erro", t, err)
				default:
					fmt.Fprintf(stdout, "%-7s %-6s  %s  (%s)\n", f.status, t, orDash(f.Version), f.Path)
					if t == deps.YTDLP {
						ytdlp = &f.Found
					}
				}
			}

			if ytdlp != nil {
				if age, ok := deps.YTDLPAge(ytdlp.Version, d.now()); ok && age > deps.MaxYTDLPAge {
					fmt.Fprintf(stderr, "aviso: o yt-dlp tem %d dias e pode não funcionar com o YouTube; rode spotgo setup --update\n",
						int(age.Hours()/24))
				}
			}
			if failed > 0 {
				return fmt.Errorf("%d de %d ferramentas falharam", failed, len(deps.Tools))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&update, "update", false, "baixa de novo as versões mais recentes")
	return cmd
}

type setupResult struct {
	deps.Found
	status string
}

func setupTool(ctx context.Context, d setupDeps, t deps.Tool, update bool) (setupResult, error) {
	all, err := d.locate(ctx, t)
	if err != nil && !errors.Is(err, deps.ErrMissing) {
		return setupResult{}, err
	}
	var managed, system *deps.Found
	for i := range all {
		switch all[i].Origin {
		case deps.FromManaged:
			managed = &all[i]
		case deps.FromPath:
			if system == nil {
				system = &all[i]
			}
		}
	}

	if !update {
		if managed != nil {
			return setupResult{*managed, "ok"}, nil
		}
		if system != nil && usable(t, *system, d.now()) {
			return setupResult{*system, "ok"}, nil
		}
	}

	path, err := d.install(ctx, t)
	if err != nil {
		// Sem build oficial (ffmpeg no macOS): o do sistema, se houver, serve.
		if errors.Is(err, deps.ErrUnsupported) {
			if managed != nil {
				return setupResult{*managed, "ok"}, nil
			}
			if system != nil {
				return setupResult{*system, "ok"}, nil
			}
		}
		return setupResult{}, err
	}
	f := deps.Found{Tool: t, Path: path, Origin: deps.FromManaged, Version: d.version(ctx, t, path)}
	return setupResult{f, "baixado"}, nil
}

// usable: um yt-dlp do sistema velho (ou de versão ilegível) quebra com o YouTube.
func usable(t deps.Tool, f deps.Found, now time.Time) bool {
	if t != deps.YTDLP {
		return true
	}
	age, ok := deps.YTDLPAge(f.Version, now)
	return ok && age <= deps.MaxYTDLPAge
}

func downloadProgress(w io.Writer) func(t deps.Tool, done, total int64) {
	f, ok := w.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return nil
	}
	var last int64
	return func(t deps.Tool, done, total int64) {
		if done < last {
			last = 0
		}
		if done-last < 256<<10 && done != total {
			return
		}
		last = done
		if total > 0 {
			fmt.Fprintf(w, "\rbaixando %s: %.1f/%.1f MB", t, mb(done), mb(total))
		} else {
			fmt.Fprintf(w, "\rbaixando %s: %.1f MB", t, mb(done))
		}
	}
}

func mb(n int64) float64 { return float64(n) / (1 << 20) }
