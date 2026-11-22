package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DeathHapyness/spotGO/internal/embed"
	"github.com/DeathHapyness/spotGO/internal/song"
	"github.com/DeathHapyness/spotGO/internal/source"
)

const (
	idBom = "0ETFjACtuP2ADo6LFhL6HN"
	id404 = "0000000000000000000000"
)

type fakeResolver struct {
	results map[string]source.Result
	errs    map[string]error
}

func (f *fakeResolver) Resolve(_ context.Context, ref song.Ref) (source.Result, error) {
	if err := f.errs[ref.ID]; err != nil {
		return source.Result{}, err
	}
	r := f.results[ref.ID]
	r.Ref = ref
	return r, nil
}

func abbeyRoad() source.Result {
	return source.Result{
		Name:   "Abbey Road (Remastered 2009)",
		Artist: "The Beatles",
		Songs: []song.Song{
			{Title: "Come Together - Remastered 2009", Artists: []string{"The Beatles"}, Album: "Abbey Road (Remastered 2009)",
				Duration: 259946 * time.Millisecond, ISRC: "GBAYE0601690", TrackNumber: 1},
			{Title: "Faixa Solta", Artists: []string{"A", "B"}, Duration: 61 * time.Second},
		},
	}
}

func runSave(t *testing.T, r resolver, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := newSaveCmd(func(func(int, int)) resolver { return r })
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	err = cmd.ExecuteContext(context.Background())
	return out.String(), errb.String(), err
}

func albumURL(id string) string { return "https://open.spotify.com/album/" + id }

func TestSaveText(t *testing.T) {
	r := &fakeResolver{results: map[string]source.Result{idBom: abbeyRoad()}}
	out, errOut, err := runSave(t, r, albumURL(idBom))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Abbey Road (Remastered 2009) — The Beatles  [álbum, 2 faixas]",
		"  1. Come Together - Remastered 2009 — The Beatles | Abbey Road (Remastered 2009) | 4:19 | GBAYE0601690",
		"  2. Faixa Solta — A, B | - | 1:01 | -",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("faltou %q em:\n%s", want, out)
		}
	}
	if errOut != "" {
		t.Errorf("stderr = %q, quero vazio", errOut)
	}
}

func TestSaveWarnings(t *testing.T) {
	res := abbeyRoad()
	res.Unmatched, res.Failed, res.Truncated = 1, 1, true
	r := &fakeResolver{results: map[string]source.Result{idBom: res}}
	out, errOut, err := runSave(t, r, albumURL(idBom))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"aviso: 1 de 2 faixas ficaram sem dados do Deezer",
		"aviso: 1 consultas ao Deezer falharam por erro de rede",
		"aviso: o Spotify só mostra 100 faixas desta playlist",
	} {
		if !strings.Contains(errOut, want) {
			t.Errorf("faltou %q no stderr:\n%s", want, errOut)
		}
	}
	if strings.Contains(out, "aviso:") {
		t.Errorf("aviso no stdout:\n%s", out)
	}
}

func TestSaveJSON(t *testing.T) {
	res := abbeyRoad()
	res.Unmatched = 1
	r := &fakeResolver{results: map[string]source.Result{idBom: res}}
	out, _, err := runSave(t, r, "--json", albumURL(idBom))
	if err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("JSON inválido: %v\n%s", err, out)
	}
	if len(got) != 1 || got[0]["kind"] != "album" || got[0]["id"] != idBom || got[0]["name"] != res.Name {
		t.Fatalf("objeto = %v", got)
	}
	songs := got[0]["songs"].([]any)
	s0, s1 := songs[0].(map[string]any), songs[1].(map[string]any)
	if s0["duration_ms"] != float64(259946) || s0["isrc"] != "GBAYE0601690" || s0["track_number"] != float64(1) {
		t.Fatalf("songs[0] = %v", s0)
	}
	if _, ok := s1["isrc"]; ok {
		t.Fatalf("songs[1] tem isrc vazio: %v", s1)
	}
	if _, ok := s1["track_number"]; ok {
		t.Fatalf("songs[1] tem track_number zero: %v", s1)
	}
}

func TestSavePartialFailure(t *testing.T) {
	r := &fakeResolver{
		results: map[string]source.Result{idBom: abbeyRoad()},
		errs:    map[string]error{id404: fmt.Errorf("%w: x", embed.ErrNotFound)},
	}
	out, errOut, err := runSave(t, r, albumURL(idBom), "https://exemplo.com/x", albumURL(id404))
	if err == nil || !strings.Contains(err.Error(), "2 de 3") {
		t.Fatalf("err = %v, quero \"2 de 3 links falharam\"", err)
	}
	if !strings.Contains(out, "Abbey Road") {
		t.Errorf("o link bom não foi mostrado:\n%s", out)
	}
	if n := strings.Count(errOut, "erro: "); n != 2 {
		t.Errorf("linhas de erro = %d, quero 2:\n%s", n, errOut)
	}
}

func TestSaveNoArgs(t *testing.T) {
	if _, _, err := runSave(t, &fakeResolver{}); err == nil {
		t.Fatal("sem argumentos deveria dar erro")
	}
}

func TestRootHasSave(t *testing.T) {
	cmd, _, err := newRootCmd().Find([]string{"save"})
	if err != nil || cmd.Name() != "save" {
		t.Fatalf("comando save não registrado: %v", err)
	}
}
