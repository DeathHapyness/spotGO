package syncstate

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"strings"
	"sync"
)

// Archive é a lista de faixas já baixadas (uma por linha, "spotify <id>",
// como o --download-archive do yt-dlp). Faixas nela nem entram na busca.
type Archive struct {
	path string
	mu   sync.Mutex
	ids  map[string]bool
}

func OpenArchive(path string) (*Archive, error) {
	a := &Archive{path: path, ids: map[string]bool{}}
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return a, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if id, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "spotify "); ok && id != "" {
			a.ids[id] = true
		}
	}
	return a, sc.Err()
}

func (a *Archive) Has(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return id != "" && a.ids[id]
}

func (a *Archive) Add(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if id == "" || a.ids[id] {
		return nil
	}
	f, err := os.OpenFile(a.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = f.WriteString("spotify " + id + "\n")
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		a.ids[id] = true
	}
	return err
}
