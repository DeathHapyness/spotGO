package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DeathHapyness/spotGO/internal/deps"
)

type fakeSetup struct {
	found     map[deps.Tool][]deps.Found
	installed []deps.Tool
	failWith  map[deps.Tool]error
	newVer    map[deps.Tool]string
}

func (f *fakeSetup) deps() setupDeps {
	return setupDeps{
		locate: func(_ context.Context, t deps.Tool) ([]deps.Found, error) {
			if fs := f.found[t]; len(fs) > 0 {
				return fs, nil
			}
			return nil, fmt.Errorf("%s: %w", t, deps.ErrMissing)
		},
		install: func(_ context.Context, t deps.Tool) (string, error) {
			if err := f.failWith[t]; err != nil {
				return "", err
			}
			f.installed = append(f.installed, t)
			return "/managed/" + string(t), nil
		},
		version: func(_ context.Context, t deps.Tool, _ string) string {
			if v, ok := f.newVer[t]; ok {
				return v
			}
			return "2026.11.20"
		},
		now: func() time.Time { return time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC) },
	}
}

func managed(t deps.Tool, v string) deps.Found {
	return deps.Found{Tool: t, Path: "/managed/" + string(t), Origin: deps.FromManaged, Version: v}
}

func system(t deps.Tool, v string) deps.Found {
	return deps.Found{Tool: t, Path: "/usr/bin/" + string(t), Origin: deps.FromPath, Version: v}
}

func runSetup(t *testing.T, f *fakeSetup, args ...string) (string, string, error) {
	t.Helper()
	cmd := newSetupCmd(func(func(deps.Tool, int64, int64)) setupDeps { return f.deps() })
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	return out.String(), errb.String(), err
}

func countPrefix(s, prefix string) int {
	n := 0
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(l, prefix) {
			n++
		}
	}
	return n
}

func TestSetupInstallsMissing(t *testing.T) {
	f := &fakeSetup{}
	out, _, err := runSetup(t, f)
	if err != nil || len(f.installed) != 3 || countPrefix(out, "baixado") != 3 {
		t.Fatalf("err = %v, instalados = %v, saída:\n%s", err, f.installed, out)
	}
}

func TestSetupSkipsPresent(t *testing.T) {
	f := &fakeSetup{found: map[deps.Tool][]deps.Found{
		deps.YTDLP: {managed(deps.YTDLP, "2026.11.20")}, deps.FFmpeg: {managed(deps.FFmpeg, "n9.0")}, deps.Deno: {managed(deps.Deno, "2.9.8")},
	}}
	out, _, err := runSetup(t, f)
	if err != nil || len(f.installed) != 0 || countPrefix(out, "ok") != 3 || !strings.Contains(out, "/managed/ffmpeg") {
		t.Fatalf("err = %v, instalados = %v, saída:\n%s", err, f.installed, out)
	}
}

func TestSetupOldSystemYTDLP(t *testing.T) {
	f := &fakeSetup{found: map[deps.Tool][]deps.Found{
		deps.YTDLP: {system(deps.YTDLP, "2024.04.09")}, deps.FFmpeg: {system(deps.FFmpeg, "6.1.1")},
	}}
	out, errOut, err := runSetup(t, f)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(toStrings(f.installed), ",") != "yt-dlp,deno" {
		t.Fatalf("instalados = %v", f.installed)
	}
	if !strings.Contains(out, "/usr/bin/ffmpeg") || strings.Contains(errOut, "setup --update") {
		t.Fatalf("saída:\n%s\nstderr:\n%s", out, errOut)
	}
}

func TestSetupUpdate(t *testing.T) {
	f := &fakeSetup{found: map[deps.Tool][]deps.Found{
		deps.YTDLP: {managed(deps.YTDLP, "2026.11.20")}, deps.FFmpeg: {managed(deps.FFmpeg, "n9.0")}, deps.Deno: {managed(deps.Deno, "2.9.8")},
	}}
	if _, _, err := runSetup(t, f, "--update"); err != nil || len(f.installed) != 3 {
		t.Fatalf("err = %v, instalados = %v", err, f.installed)
	}
}

func TestSetupUnsupported(t *testing.T) {
	f := &fakeSetup{failWith: map[deps.Tool]error{
		deps.FFmpeg: fmt.Errorf("ffmpeg em darwin/arm64: %w; instale com brew install ffmpeg", deps.ErrUnsupported),
	}}
	out, _, err := runSetup(t, f)
	if err != nil || !strings.Contains(out, "brew install ffmpeg") || len(f.installed) != 2 {
		t.Fatalf("err = %v, instalados = %v, saída:\n%s", err, f.installed, out)
	}
}

func TestSetupUnsupportedButInPath(t *testing.T) {
	f := &fakeSetup{
		found:    map[deps.Tool][]deps.Found{deps.FFmpeg: {system(deps.FFmpeg, "8.0")}},
		failWith: map[deps.Tool]error{deps.FFmpeg: fmt.Errorf("ffmpeg: %w", deps.ErrUnsupported)},
	}
	out, _, err := runSetup(t, f, "--update")
	if err != nil || !strings.Contains(out, "/usr/bin/ffmpeg") {
		t.Fatalf("err = %v, saída:\n%s", err, out)
	}
}

func TestSetupFailure(t *testing.T) {
	f := &fakeSetup{failWith: map[deps.Tool]error{deps.Deno: errors.New("conexão recusada")}}
	out, _, err := runSetup(t, f)
	if err == nil || !strings.Contains(err.Error(), "1 de 3") || len(f.installed) != 2 || !strings.Contains(out, "conexão recusada") {
		t.Fatalf("err = %v, instalados = %v, saída:\n%s", err, f.installed, out)
	}
}

func TestSetupStaleWarning(t *testing.T) {
	f := &fakeSetup{found: map[deps.Tool][]deps.Found{
		deps.YTDLP: {managed(deps.YTDLP, "2026.08.20")}, deps.FFmpeg: {managed(deps.FFmpeg, "n9.0")}, deps.Deno: {managed(deps.Deno, "2.9.8")},
	}}
	_, errOut, err := runSetup(t, f)
	if err != nil || !strings.Contains(errOut, "spotgo setup --update") || !strings.Contains(errOut, "94 dias") {
		t.Fatalf("err = %v, stderr = %q", err, errOut)
	}
}

func TestSetupCanceled(t *testing.T) {
	f := &fakeSetup{failWith: map[deps.Tool]error{deps.YTDLP: context.Canceled}}
	_, _, err := runSetup(t, f)
	if err == nil || !strings.Contains(err.Error(), "interrompido") {
		t.Fatalf("err = %v", err)
	}
}

func toStrings(ts []deps.Tool) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = string(t)
	}
	return out
}
