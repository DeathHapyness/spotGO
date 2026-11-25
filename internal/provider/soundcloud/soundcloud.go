// Package soundcloud busca no SoundCloud pelo próprio yt-dlp (scsearch), sem
// extrair o client_id do site no nosso código.
package soundcloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/DeathHapyness/spotGO/internal/matching"
	"github.com/DeathHapyness/spotGO/internal/provider"
	"github.com/DeathHapyness/spotGO/internal/song"
)

const limit = 5

type Client struct {
	YTDLP string
	Run   func(ctx context.Context, path string, args []string) ([]byte, error)
}

type entry struct {
	URL       string  `json:"url"`
	Title     string  `json:"title"`
	Uploader  string  `json:"uploader"`
	Duration  float64 `json:"duration"`
	ViewCount int64   `json:"view_count"`
	ID        string  `json:"id"`
}

func parse(body []byte) ([]matching.Result, error) {
	var p struct {
		Entries []entry `json:"entries"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("resposta do yt-dlp para o SoundCloud ilegível: %w", err)
	}
	var out []matching.Result
	for _, e := range p.Entries {
		if e.URL == "" || e.Duration <= 0 {
			continue
		}
		out = append(out, matching.Result{
			Source:   "soundcloud",
			ID:       e.ID,
			URL:      e.URL,
			Name:     e.Title,
			Author:   e.Uploader,
			Duration: time.Duration(e.Duration * float64(time.Second)),
			Views:    e.ViewCount,
		})
	}
	return out, nil
}

func (c *Client) Search(ctx context.Context, query string) ([]matching.Result, error) {
	run := c.Run
	if run == nil {
		run = runOutput
	}
	out, err := run(ctx, c.YTDLP, []string{
		"--ignore-config", "--no-warnings", "--flat-playlist", "-J",
		fmt.Sprintf("scsearch%d:%s", limit, query),
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("busca no SoundCloud %q: %w", query, err)
	}
	return parse(out)
}

func (c *Client) Find(ctx context.Context, s song.Song) (matching.Scored, error) {
	rs, err := c.Search(ctx, provider.SimpleQuery(s))
	if err != nil {
		return matching.Scored{}, err
	}
	return provider.Pick(s, rs, "SoundCloud")
}

func runOutput(ctx context.Context, path string, args []string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		msg := strings.TrimSpace(stderr.String())
		if i := strings.LastIndexByte(msg, '\n'); i >= 0 {
			msg = msg[i+1:]
		}
		err = fmt.Errorf("yt-dlp saiu com código %d: %s", exitErr.ExitCode(), msg)
	}
	return stdout.Bytes(), err
}
