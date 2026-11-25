package output

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type M3UEntry struct {
	// Path absoluto ou relativo ao diretório atual; no arquivo vai relativo ao .m3u8.
	Path     string
	Title    string
	Duration time.Duration
}

// WriteM3U grava uma playlist M3U8 (UTF-8) de forma atômica.
func WriteM3U(path string, entries []M3UEntry) error {
	base, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	for _, e := range entries {
		p := e.Path
		if abs, err := filepath.Abs(p); err == nil {
			if rel, err := filepath.Rel(base, abs); err == nil {
				p = rel
			}
		}
		secs := int(e.Duration.Round(time.Second).Seconds())
		if secs <= 0 {
			secs = -1
		}
		title := strings.ReplaceAll(e.Title, "\n", " ")
		fmt.Fprintf(&b, "#EXTINF:%d,%s\n%s\n", secs, title, filepath.ToSlash(p))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.WriteString(b.String())
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), 0o644)
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
