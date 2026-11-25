package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/DeathHapyness/spotGO/internal/config"
	"github.com/DeathHapyness/spotGO/internal/deezer"
	"github.com/DeathHapyness/spotGO/internal/deps"
	"github.com/DeathHapyness/spotGO/internal/download"
	"github.com/DeathHapyness/spotGO/internal/embed"
	"github.com/DeathHapyness/spotGO/internal/fetch"
	"github.com/DeathHapyness/spotGO/internal/ffmpeg"
	"github.com/DeathHapyness/spotGO/internal/lyrics"
	"github.com/DeathHapyness/spotGO/internal/provider"
	"github.com/DeathHapyness/spotGO/internal/provider/bandcamp"
	"github.com/DeathHapyness/spotGO/internal/provider/soundcloud"
	"github.com/DeathHapyness/spotGO/internal/provider/youtube"
	"github.com/DeathHapyness/spotGO/internal/provider/ytmusic"
	"github.com/DeathHapyness/spotGO/internal/song"
	"github.com/DeathHapyness/spotGO/internal/source"
	"github.com/DeathHapyness/spotGO/internal/spotify"
)

// O embed do Spotify responde diferente para clientes que não parecem um navegador.
const browserUserAgent = "Mozilla/5.0 (X11; Linux x86_64) Chrome/131.0 Safari/537.36"

type resolver interface {
	Resolve(ctx context.Context, ref song.Ref) (source.Result, error)
}

// newPublicResolver monta o modo público com um único fetch.Client, para o
// cache e os limites por host valerem para o embed e o Deezer juntos.
func newPublicResolver(progress func(done, total int)) *source.Public {
	f := newFetch()
	p := source.NewPublic(embed.New(f), deezer.New(f))
	p.Progress = progress
	return p
}

// newURLFinder só procura o yt-dlp (sem instalar nada) se o SoundCloud foi pedido.
func newURLFinder(audio []string) (finder, error) {
	ytdlp := ""
	if slices.Contains(audio, "soundcloud") {
		dir, _ := deps.ManagedDir()
		f, err := (&deps.Locator{Dir: dir}).Locate(context.Background(), deps.YTDLP)
		if err != nil {
			return nil, fmt.Errorf("o SoundCloud precisa do yt-dlp: %w; rode spotgo setup", err)
		}
		ytdlp = f.Path
	}
	return newChain(newFetch(), audio, ytdlp), nil
}

func newSetupDeps(progress func(t deps.Tool, done, total int64)) setupDeps {
	dir, err := deps.ManagedDir()
	if err != nil {
		dir = ""
	}
	in := &deps.Installer{Dir: dir, Progress: progress}
	loc := &deps.Locator{Dir: dir}
	return setupDeps{
		locate: loc.LocateAll,
		install: func(ctx context.Context, t deps.Tool) (string, error) {
			if dir == "" {
				return "", err
			}
			return in.Install(ctx, t)
		},
		version: loc.Version,
		now:     time.Now,
	}
}

func newDownloadEnv(progress func(done, total int)) downloadEnv {
	f := newFetch()
	p := source.NewPublic(embed.New(f), deezer.New(f))
	p.Progress = progress
	return downloadEnv{
		resolver: p,
		newFinder: func(audio []string, ytdlp string) (download.Finder, error) {
			return newChain(f, audio, ytdlp), nil
		},
		covers: coverFetcher{f},
		lyrics: lyrics.New(f),
		tools:  downloadTools,
	}
}

func newChain(f *fetch.Client, names []string, ytdlp string) provider.Chain {
	var c provider.Chain
	for _, n := range names {
		var p provider.Provider
		switch n {
		case "youtube-music":
			p = ytmusic.New(f)
		case "youtube":
			p = youtube.New(f)
		case "soundcloud":
			p = &soundcloud.Client{YTDLP: ytdlp}
		case "bandcamp":
			p = bandcamp.New(f)
		}
		c = append(c, provider.Named{Name: n, Provider: p})
	}
	return c
}

func newMetaEnv(progress func(done, total int)) metaEnv {
	f := newFetch()
	dz := deezer.New(f)
	return metaEnv{
		resolver: source.NewPublic(embed.New(f), dz),
		enrich:   dz.EnrichTrack,
		covers:   coverFetcher{f},
		lyrics:   lyrics.New(f),
		tagger: func(path string) (tagger, error) {
			dir, _ := deps.ManagedDir()
			loc := &deps.Locator{Dir: dir, Flags: map[deps.Tool]string{deps.FFmpeg: path}}
			found, err := loc.Locate(context.Background(), deps.FFmpeg)
			if errors.Is(err, deps.ErrMissing) {
				return nil, errors.New("o ffmpeg não foi encontrado; rode spotgo setup")
			}
			if err != nil {
				return nil, err
			}
			return &ffmpeg.Converter{Path: found.Path}, nil
		},
	}
}

type coverFetcher struct{ f *fetch.Client }

func (c coverFetcher) Cover(ctx context.Context, url string) ([]byte, error) {
	return c.f.Get(ctx, fetch.Request{URL: url, CacheTTL: 30 * 24 * time.Hour})
}

func downloadTools(ctx context.Context, fl downloadFlags, confirm func([]deps.Tool) error) (download.Fetcher, download.Converter, []string, error) {
	dir, dirErr := deps.ManagedDir()
	loc := &deps.Locator{Dir: dir, Flags: map[deps.Tool]string{}}
	for t, p := range fl.tools {
		loc.Flags[t] = *p
	}
	in := &deps.Installer{Dir: dir}
	install := func(ctx context.Context, t deps.Tool) (string, error) {
		if dirErr != nil {
			return "", dirErr
		}
		return in.Install(ctx, t)
	}
	found, warnings, err := ensureTools(ctx, loc, install, confirm, time.Now())
	if err != nil {
		return nil, nil, nil, err
	}
	y := &download.YTDLP{Path: found[deps.YTDLP].Path, Deno: found[deps.Deno].Path}
	return y, &ffmpeg.Converter{Path: found[deps.FFmpeg].Path}, warnings, nil
}

func newFetch() *fetch.Client {
	cacheDir, _ := fetch.DefaultCacheDir()
	return fetch.New(fetch.Options{UserAgent: browserUserAgent, CacheDir: cacheDir})
}

const envConfigPath = "SPOTGO_CONFIG"

var httpClient = &http.Client{Timeout: 15 * time.Second}

func configPath() (string, error) {
	if p := os.Getenv(envConfigPath); p != "" {
		return p, nil
	}
	return config.DefaultPath()
}

func stdinTerminal(cmd *cobra.Command) (*os.File, bool) {
	f, ok := cmd.InOrStdin().(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return nil, false
	}
	return f, true
}

func newPrompter(cmd *cobra.Command) config.Prompter {
	in := bufio.NewReader(cmd.InOrStdin())
	out := cmd.OutOrStdout()
	return config.Prompter{
		In:         in,
		Out:        out,
		ReadSecret: secretReader(cmd, in, out),
	}
}

func secretReader(cmd *cobra.Command, in *bufio.Reader, out io.Writer) func() (string, error) {
	return func() (string, error) {
		if f, ok := stdinTerminal(cmd); ok {
			b, err := term.ReadPassword(int(f.Fd()))
			fmt.Fprintln(out)
			return string(b), err
		}
		return in.ReadString('\n')
	}
}

func validateCredentials(ctx context.Context, c config.Credentials) error {
	_, err := spotify.RequestToken(ctx, httpClient, spotify.TokenURL, c)
	return err
}

func isInvalidCredentials(err error) bool {
	return errors.Is(err, spotify.ErrInvalidCredentials)
}
