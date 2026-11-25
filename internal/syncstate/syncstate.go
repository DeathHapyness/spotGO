// Package syncstate guarda o que o `spotgo sync` baixou numa pasta e calcula
// o que mudou desde a última vez: o que mover, baixar e apagar.
package syncstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const FileName = ".spotgo-sync.json"

type State struct {
	Links    []string `json:"links"`
	Template string   `json:"template"`
	Format   string   `json:"format"`
	Bitrate  string   `json:"bitrate,omitempty"`
	Audio    string   `json:"audio,omitempty"`
	// Files liga o ID da faixa ao caminho (relativo à pasta, com /) do arquivo
	// que o sync criou. Só esses arquivos podem ser apagados.
	Files map[string]string `json:"files"`
}

func Load(dir string) (State, bool, error) {
	b, err := os.ReadFile(filepath.Join(dir, FileName))
	if errors.Is(err, fs.ErrNotExist) {
		return State{Files: map[string]string{}}, false, nil
	}
	if err != nil {
		return State{}, false, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return State{}, false, fmt.Errorf("%s corrompido: %w", FileName, err)
	}
	if s.Files == nil {
		s.Files = map[string]string{}
	}
	for id, p := range s.Files {
		if !safe(p) {
			return State{}, false, fmt.Errorf("%s: caminho fora da pasta para %s: %q", FileName, id, p)
		}
	}
	return s, true, nil
}

// safe impede que um estado editado à mão faça o sync apagar algo fora da pasta.
func safe(rel string) bool {
	if rel == "" || filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") {
		return false
	}
	c := filepath.Clean(filepath.FromSlash(rel))
	return c != "." && c != ".." && !strings.HasPrefix(c, ".."+string(filepath.Separator))
}

func Save(dir string, s State) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+FileName+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, FileName))
}

type Move struct{ From, To string }

// Moves: faixas que continuam na lista mas mudaram de caminho (outra posição
// na playlist, por exemplo). Com outra extensão não é mover: o formato mudou
// e a faixa precisa ser convertida de novo.
func Moves(old, current map[string]string) []Move {
	var out []Move
	for id, from := range old {
		to, ok := current[id]
		if !ok || to == from || filepath.Ext(to) != filepath.Ext(from) {
			continue
		}
		out = append(out, Move{From: from, To: to})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].From < out[j].From })
	return out
}

// ApplyMoves renomeia em duas fases (tudo para um nome temporário, depois para
// o destino), para trocas como 01→02 e 02→03 não se atropelarem. Pula o que
// não existe mais ou cujo destino já está ocupado por um arquivo que não vai sair.
func ApplyMoves(dir string, moves []Move) ([]Move, error) {
	leaving := map[string]bool{}
	for _, m := range moves {
		leaving[m.From] = true
	}
	type staged struct {
		Move
		tmp string
	}
	var st []staged
	for i, m := range moves {
		from := filepath.Join(dir, filepath.FromSlash(m.From))
		if _, err := os.Stat(from); err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(m.To))); err == nil && !leaving[m.To] {
			continue
		}
		tmp := fmt.Sprintf("%s.spotgo-move-%d", from, i)
		if err := os.Rename(from, tmp); err != nil {
			return nil, err
		}
		st = append(st, staged{m, tmp})
	}
	var done []Move
	for _, s := range st {
		to := filepath.Join(dir, filepath.FromSlash(s.To))
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return done, err
		}
		if err := os.Rename(s.tmp, to); err != nil {
			return done, err
		}
		done = append(done, s.Move)
	}
	for _, m := range done {
		RemoveEmptyDirs(dir, m.From)
	}
	return done, nil
}

// Stale: arquivos antigos que podem ser apagados. Uma faixa que saiu da lista
// sai; uma que mudou de caminho só perde o antigo se a nova já existe (ok).
// Um caminho que ainda é de alguma faixa atual nunca é apagado.
func Stale(old, current map[string]string, ok map[string]bool) []string {
	inUse := map[string]bool{}
	for _, p := range current {
		inUse[p] = true
	}
	var out []string
	for id, p := range old {
		if inUse[p] {
			continue
		}
		if now, still := current[id]; still && (now == p || !ok[id]) {
			continue
		}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// RemoveEmptyDirs apaga as pastas que ficaram vazias acima de rel, sem sair de dir.
func RemoveEmptyDirs(dir, rel string) {
	d := filepath.Dir(filepath.FromSlash(rel))
	for d != "." && d != string(filepath.Separator) {
		if os.Remove(filepath.Join(dir, d)) != nil {
			return
		}
		d = filepath.Dir(d)
	}
}
