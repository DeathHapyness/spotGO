package deps

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var sampleOutput = map[Tool]string{
	YTDLP:  "2024.04.09\n",
	FFmpeg: "ffmpeg version n9.0-20261120 Copyright (c) 2000-2026 the FFmpeg developers\nbuilt with gcc\n",
	Deno:   "deno 2.9.8 (stable, release, x86_64-unknown-linux-gnu)\nv8 14.0\n",
}

func exe(t *testing.T, dir string, tool Tool) string {
	t.Helper()
	os.MkdirAll(dir, 0o755)
	p := filepath.Join(dir, BinaryName(tool, runtime.GOOS))
	if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// locator monta um Locator com PATH falso: pathHas diz quais ferramentas o PATH tem.
func locator(t *testing.T, pathHas ...Tool) (*Locator, string) {
	t.Helper()
	root := t.TempDir()
	sys := filepath.Join(root, "usr-bin")
	inPath := map[string]string{}
	for _, tool := range pathHas {
		inPath[BinaryName(tool, runtime.GOOS)] = exe(t, sys, tool)
	}
	l := &Locator{
		Dir: filepath.Join(root, "managed"),
		LookPath: func(name string) (string, error) {
			if p, ok := inPath[name]; ok {
				return p, nil
			}
			return "", errors.New("not found")
		},
		Run: func(_ context.Context, path string, _ ...string) (string, error) {
			for tool, out := range sampleOutput {
				if strings.HasPrefix(filepath.Base(path), string(tool)) {
					return out, nil
				}
			}
			return "", errors.New("?")
		},
	}
	return l, root
}

func TestLocateOrder(t *testing.T) {
	ctx := context.Background()
	l, root := locator(t, YTDLP)
	managed := exe(t, l.Dir, YTDLP)
	flag := exe(t, filepath.Join(root, "flag"), YTDLP)
	l.Flags = map[Tool]string{YTDLP: flag}

	if f, err := l.Locate(ctx, YTDLP); err != nil || f.Path != flag || f.Origin != FromFlag {
		t.Fatalf("com flag: %+v, %v", f, err)
	}
	l.Flags = nil
	if f, err := l.Locate(ctx, YTDLP); err != nil || f.Path != managed || f.Origin != FromManaged {
		t.Fatalf("gerenciado: %+v, %v", f, err)
	}
	os.Remove(managed)
	if f, err := l.Locate(ctx, YTDLP); err != nil || f.Origin != FromPath || f.Version != "2024.04.09" {
		t.Fatalf("PATH: %+v, %v", f, err)
	}
	if _, err := l.Locate(ctx, Deno); !errors.Is(err, ErrMissing) {
		t.Fatalf("ausente: %v", err)
	}
}

func TestLocateFlagMissingFile(t *testing.T) {
	l, root := locator(t, YTDLP)
	l.Flags = map[Tool]string{YTDLP: filepath.Join(root, "nao-existe")}
	if _, err := l.Locate(context.Background(), YTDLP); err == nil || !strings.Contains(err.Error(), "--yt-dlp") {
		t.Fatalf("err = %v", err)
	}
}

func TestLocateVersions(t *testing.T) {
	l, _ := locator(t)
	want := map[Tool]string{YTDLP: "2024.04.09", FFmpeg: "n9.0-20261120", Deno: "2.9.8"}
	for tool, v := range want {
		exe(t, l.Dir, tool)
		f, err := l.Locate(context.Background(), tool)
		if err != nil || f.Version != v {
			t.Errorf("%s: %+v, %v; quero versão %s", tool, f, err, v)
		}
	}
}

func TestLocateUnreadableVersion(t *testing.T) {
	l, _ := locator(t)
	l.Run = func(context.Context, string, ...string) (string, error) { return "", errors.New("exec format error") }
	exe(t, l.Dir, YTDLP)
	if f, err := l.Locate(context.Background(), YTDLP); err != nil || f.Version != "" || f.Origin != FromManaged {
		t.Fatalf("%+v, %v", f, err)
	}
}

func TestLocateSkipsNonExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sem bit de execução no Windows")
	}
	l, _ := locator(t)
	p := exe(t, l.Dir, YTDLP)
	os.Chmod(p, 0o644)
	if _, err := l.Locate(context.Background(), YTDLP); !errors.Is(err, ErrMissing) {
		t.Fatalf("err = %v", err)
	}
}

func TestLocateAll(t *testing.T) {
	l, _ := locator(t, YTDLP)
	exe(t, l.Dir, YTDLP)
	all, err := l.LocateAll(context.Background(), YTDLP)
	if err != nil || len(all) != 2 || all[0].Origin != FromManaged || all[1].Origin != FromPath {
		t.Fatalf("LocateAll = %+v, %v", all, err)
	}
}

func TestYTDLPAge(t *testing.T) {
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	if d, ok := YTDLPAge("2026.10.26", now); !ok || d.Hours()/24 < 27 || d.Hours()/24 >= 28 {
		t.Errorf("2026.10.26 = %v, %v", d, ok)
	}
	if d, ok := YTDLPAge("2024.04.09", now); !ok || d <= MaxYTDLPAge {
		t.Errorf("2024.04.09 = %v, %v", d, ok)
	}
	if _, ok := YTDLPAge("2026.10.26.1", now); !ok {
		t.Error("2026.10.26.1 recusado")
	}
	for _, bad := range []string{"abc", "", "2026.13.40"} {
		if _, ok := YTDLPAge(bad, now); ok {
			t.Errorf("%q aceito", bad)
		}
	}
}
