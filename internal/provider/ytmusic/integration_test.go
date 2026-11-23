//go:build integration

package ytmusic

import (
	"context"
	"testing"
	"time"

	"github.com/DeathHapyness/spotGO/internal/fetch"
)

func TestIntegrationFind(t *testing.T) {
	c := New(fetch.New(fetch.Options{}))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	got, err := c.Find(ctx, comeTogether)
	if err != nil {
		t.Fatal(err)
	}
	if got.Result.ID == "" || got.Score <= 80 {
		t.Fatalf("Find = %+v", got)
	}
	t.Logf("%s (%.1f) %s", got.Result.Name, got.Score, got.Result.URL)
}
