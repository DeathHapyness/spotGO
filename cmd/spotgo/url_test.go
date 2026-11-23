package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DeathHapyness/spotGO/internal/matching"
	"github.com/DeathHapyness/spotGO/internal/provider/ytmusic"
	"github.com/DeathHapyness/spotGO/internal/song"
	"github.com/DeathHapyness/spotGO/internal/source"
)

type fakeFinder struct {
	find func(ctx context.Context, s song.Song) (matching.Scored, error)
}

func (f fakeFinder) Find(ctx context.Context, s song.Song) (matching.Scored, error) {
	return f.find(ctx, s)
}

func threeSongs() source.Result {
	return source.Result{Name: "Álbum", Artist: "X", Songs: []song.Song{
		{Title: "Um", Artists: []string{"A"}},
		{Title: "Dois", Artists: []string{"B", "C"}},
		{Title: "Três", Artists: []string{"D"}},
	}}
}

func found(s song.Song) (matching.Scored, error) {
	return matching.Scored{Result: matching.Result{URL: "https://music.youtube.com/watch?v=" + s.Title}, Score: 90}, nil
}

func runURL(t *testing.T, ctx context.Context, f finder, args ...string) (string, string, error) {
	t.Helper()
	r := &fakeResolver{results: map[string]source.Result{idBom: threeSongs()}}
	cmd := newURLCmd(func(func(int, int)) resolver { return r }, func() finder { return f })
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(ctx)
	return out.String(), errb.String(), err
}

func TestURLPrintsInOrder(t *testing.T) {
	delay := map[string]time.Duration{"Um": 30 * time.Millisecond, "Dois": 15 * time.Millisecond, "Três": 0}
	f := fakeFinder{func(_ context.Context, s song.Song) (matching.Scored, error) {
		time.Sleep(delay[s.Title])
		return found(s)
	}}
	out, _, err := runURL(t, context.Background(), f, albumURL(idBom))
	if err != nil {
		t.Fatal(err)
	}
	want := "A - Um\thttps://music.youtube.com/watch?v=Um\n" +
		"B, C - Dois\thttps://music.youtube.com/watch?v=Dois\n" +
		"D - Três\thttps://music.youtube.com/watch?v=Três\n"
	if out != want {
		t.Fatalf("saída:\n%s\nquero:\n%s", out, want)
	}
}

func TestURLNotFoundContinues(t *testing.T) {
	f := fakeFinder{func(_ context.Context, s song.Song) (matching.Scored, error) {
		if s.Title == "Dois" {
			return matching.Scored{}, ytmusic.ErrNotFound
		}
		return found(s)
	}}
	out, errOut, err := runURL(t, context.Background(), f, albumURL(idBom))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "B, C - Dois\tnão encontrada\n") || !strings.Contains(out, "D - Três\thttps://") {
		t.Fatalf("saída = %q", out)
	}
	if !strings.Contains(errOut, "2 de 3 faixas encontradas") {
		t.Fatalf("stderr = %q", errOut)
	}
}

func TestURLSearchErrorFails(t *testing.T) {
	f := fakeFinder{func(_ context.Context, s song.Song) (matching.Scored, error) {
		if s.Title == "Um" {
			return matching.Scored{}, errors.New("rede caiu")
		}
		return found(s)
	}}
	out, errOut, err := runURL(t, context.Background(), f, albumURL(idBom))
	if err == nil || !strings.Contains(errOut, "rede caiu") || !strings.Contains(out, "D - Três\thttps://") {
		t.Fatalf("err = %v, stderr = %q, saída = %q", err, errOut, out)
	}
	if strings.Contains(out, "A - Um") {
		t.Fatalf("faixa com erro saiu no stdout: %q", out)
	}
}

func TestURLJSON(t *testing.T) {
	f := fakeFinder{func(_ context.Context, s song.Song) (matching.Scored, error) {
		if s.Title == "Dois" {
			return matching.Scored{}, ytmusic.ErrNotFound
		}
		return found(s)
	}}
	out, _, err := runURL(t, context.Background(), f, "--json", albumURL(idBom))
	if err != nil {
		t.Fatal(err)
	}
	var got []struct {
		Title   string   `json:"title"`
		Artists []string `json:"artists"`
		URL     string   `json:"url"`
		Score   float64  `json:"score"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("JSON inválido: %v\n%s", err, out)
	}
	if len(got) != 3 || got[0].URL == "" || got[0].Score != 90 || got[1].URL != "" || got[1].Title != "Dois" || len(got[1].Artists) != 2 {
		t.Fatalf("JSON = %+v", got)
	}
}

func TestURLCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := fakeFinder{func(ctx context.Context, s song.Song) (matching.Scored, error) {
		if s.Title == "Dois" {
			cancel()
			return matching.Scored{}, ctx.Err()
		}
		<-ctx.Done()
		return matching.Scored{}, ctx.Err()
	}}
	out, _, err := runURL(t, ctx, f, albumURL(idBom))
	if err == nil || !strings.Contains(err.Error(), "interrompido") || out != "" {
		t.Fatalf("err = %v, saída = %q", err, out)
	}
}
