package soundcloud

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/DeathHapyness/spotGO/internal/song"
)

func TestFind(t *testing.T) {
	fixture, err := os.ReadFile("testdata/come_together.json")
	if err != nil {
		t.Fatal(err)
	}
	var gotArgs []string
	c := &Client{YTDLP: "yt-dlp", Run: func(ctx context.Context, path string, args []string) ([]byte, error) {
		gotArgs = args
		return fixture, nil
	}}
	s := song.Song{Title: "Come Together", Artists: []string{"The Beatles"}, Duration: 272 * time.Second}
	got, err := c.Find(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(gotArgs, func(a string) bool { return a == "scsearch5:The Beatles Come Together" }) {
		t.Errorf("args = %v", gotArgs)
	}
	// O trecho de 43 s (prévia do Go+) e os remixes longos não podem ganhar.
	if got.Result.ID != "308477608" && got.Result.ID != "166254800" {
		t.Errorf("escolhido: %+v", got.Result)
	}
}

func TestSearchError(t *testing.T) {
	c := &Client{Run: func(context.Context, string, []string) ([]byte, error) {
		return nil, errors.New("yt-dlp saiu com código 1: ERROR: Unable to download")
	}}
	if _, err := c.Find(context.Background(), song.Song{Title: "x"}); err == nil {
		t.Error("esperava erro")
	}
	if _, err := parse([]byte("{")); err == nil {
		t.Error("JSON quebrado deve dar erro")
	}
}
