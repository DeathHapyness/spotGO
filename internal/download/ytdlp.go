package download

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DeathHapyness/spotGO/internal/ffmpeg"
)

// YTDLP baixa só o áudio, sem pós-processamento: a conversão é do ffmpeg.
type YTDLP struct {
	Path string
	// Deno é passado ao yt-dlp, que precisa de um runtime JS para o YouTube.
	Deno string
	Run  func(ctx context.Context, path string, args []string) (stdout, stderr []byte, err error)
}

// Pedir o codec que o formato final usa deixa o ffmpeg só copiar o áudio.
var preferred = map[ffmpeg.Format]string{
	ffmpeg.Opus: "bestaudio[acodec=opus]/bestaudio/best",
	ffmpeg.M4A:  "bestaudio[ext=m4a]/bestaudio/best",
}

func (y *YTDLP) Args(url string, f ffmpeg.Format, dir string) []string {
	sel, ok := preferred[f]
	if !ok {
		sel = "bestaudio/best"
	}
	args := []string{
		"--ignore-config", "--no-playlist", "--no-progress", "--no-warnings",
		"-f", sel,
		"-o", filepath.Join(dir, "%(id)s.%(ext)s"),
		"--print", "after_move:%(filepath)s\t%(acodec)s",
	}
	if y.Deno != "" {
		args = append(args, "--js-runtimes", "deno:"+y.Deno)
	}
	return append(args, "--", url)
}

func (y *YTDLP) Fetch(ctx context.Context, url string, f ffmpeg.Format, dir string) (string, string, error) {
	run := y.Run
	if run == nil {
		run = runCmd
	}
	stdout, stderr, err := run(ctx, y.Path, y.Args(url, f, dir))
	if err != nil {
		if ctx.Err() != nil {
			return "", "", ctx.Err()
		}
		return "", "", fmt.Errorf("yt-dlp falhou em %s: %w%s", url, err, lastLine(stderr))
	}
	line := strings.TrimSpace(string(stdout))
	if i := strings.LastIndexByte(line, '\n'); i >= 0 {
		line = line[i+1:]
	}
	file, codec, _ := strings.Cut(line, "\t")
	if file == "" {
		return "", "", fmt.Errorf("yt-dlp não disse onde salvou %s", url)
	}
	return file, codec, nil
}

func runCmd(ctx context.Context, path string, args []string) ([]byte, []byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		err = fmt.Errorf("saiu com código %d", exitErr.ExitCode())
	}
	return stdout.Bytes(), stderr.Bytes(), err
}

func lastLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if s == "" {
		return ""
	}
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	return ": " + s
}
