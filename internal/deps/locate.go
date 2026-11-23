package deps

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Origin string

const (
	FromFlag    Origin = "flag"
	FromManaged Origin = "spotgo"
	FromPath    Origin = "PATH"
)

type Found struct {
	Tool    Tool
	Path    string
	Origin  Origin
	Version string
}

var ErrMissing = errors.New("não encontrado")

const MaxYTDLPAge = 60 * 24 * time.Hour

const versionTimeout = 10 * time.Second

type Locator struct {
	Dir      string
	Flags    map[Tool]string
	LookPath func(string) (string, error)
	Run      func(ctx context.Context, path string, args ...string) (string, error)
}

// Locate escolhe o binário na ordem flag → pasta gerenciada → PATH.
func (l *Locator) Locate(ctx context.Context, t Tool) (Found, error) {
	all, err := l.LocateAll(ctx, t)
	if err != nil {
		return Found{}, err
	}
	return all[0], nil
}

// LocateAll devolve todos os candidatos, na ordem de prioridade.
func (l *Locator) LocateAll(ctx context.Context, t Tool) ([]Found, error) {
	var all []Found
	if p := l.Flags[t]; p != "" {
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("--%s: %s não existe", t, p)
		}
		all = append(all, Found{Tool: t, Path: p, Origin: FromFlag})
	}
	name := BinaryName(t, runtime.GOOS)
	if l.Dir != "" {
		p := filepath.Join(l.Dir, name)
		if executable(p) {
			all = append(all, Found{Tool: t, Path: p, Origin: FromManaged})
		}
	}
	lookPath := l.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if p, err := lookPath(name); err == nil {
		all = append(all, Found{Tool: t, Path: p, Origin: FromPath})
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("%s: %w", t, ErrMissing)
	}
	for i := range all {
		all[i].Version = l.Version(ctx, t, all[i].Path)
	}
	return all, nil
}

func executable(p string) bool {
	st, err := os.Stat(p)
	if err != nil || !st.Mode().IsRegular() {
		return false
	}
	return runtime.GOOS == "windows" || st.Mode().Perm()&0o111 != 0
}

// Version lê a versão do binário; "" quando não dá para ler.
func (l *Locator) Version(ctx context.Context, t Tool, path string) string {
	arg := "--version"
	if t == FFmpeg {
		arg = "-version"
	}
	run := l.Run
	if run == nil {
		run = runOutput
	}
	out, err := run(ctx, path, arg)
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	fields := strings.Fields(line)
	switch {
	case t == YTDLP && len(fields) > 0:
		return fields[0]
	case t == FFmpeg && len(fields) > 2 && fields[0] == "ffmpeg":
		return fields[2]
	case t == Deno && len(fields) > 1 && fields[0] == "deno":
		return fields[1]
	}
	return ""
}

func runOutput(ctx context.Context, path string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, args...).Output()
	return string(out), err
}

// YTDLPAge lê a versão do yt-dlp ("2026.10.26" ou "2026.10.26.1"), que é a data da release.
func YTDLPAge(version string, now time.Time) (time.Duration, bool) {
	parts := strings.Split(version, ".")
	if len(parts) < 3 {
		return 0, false
	}
	d, err := time.Parse("2006.01.02", strings.Join(parts[:3], "."))
	if err != nil {
		return 0, false
	}
	return now.Sub(d), true
}
