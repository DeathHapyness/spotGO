package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/DeathHapyness/spotGO/internal/matching"
	"github.com/DeathHapyness/spotGO/internal/song"
)

type fake struct {
	url string
	err error
	n   *int
}

func (f fake) Find(context.Context, song.Song) (matching.Scored, error) {
	*f.n++
	return matching.Scored{Result: matching.Result{URL: f.url}}, f.err
}

func TestChain(t *testing.T) {
	var calls int
	notFound := fmt.Errorf("%w no A", ErrNotFound)
	c := Chain{
		{"a", fake{err: notFound, n: &calls}},
		{"b", fake{err: errors.New("rede caiu"), n: &calls}},
		{"c", fake{url: "achou", n: &calls}},
		{"d", fake{url: "nunca", n: &calls}},
	}
	got, err := c.Find(context.Background(), song.Song{})
	if err != nil || got.Result.URL != "achou" || calls != 3 {
		t.Fatalf("got=%v err=%v calls=%d", got, err, calls)
	}

	_, err = Chain{{"a", fake{err: notFound, n: &calls}}, {"b", fake{err: notFound, n: &calls}}}.Find(context.Background(), song.Song{})
	if !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "a, b") {
		t.Errorf("err = %v", err)
	}

	_, err = Chain{{"a", fake{err: notFound, n: &calls}}, {"b", fake{err: errors.New("rede caiu"), n: &calls}}}.Find(context.Background(), song.Song{})
	if err == nil || errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "b: rede caiu") {
		t.Errorf("erro de rede não pode virar 'não achada': %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls = 0
	_, err = Chain{{"a", fake{err: errors.New("x"), n: &calls}}, {"b", fake{url: "y", n: &calls}}}.Find(ctx, song.Song{})
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Errorf("cancelado: err=%v calls=%d", err, calls)
	}
}

func TestSimpleQuery(t *testing.T) {
	s := song.Song{Title: "Here Comes The Sun - Remastered 2009", Artists: []string{"The Beatles", "X"}}
	if got := SimpleQuery(s); got != "The Beatles Here Comes The Sun" {
		t.Errorf("SimpleQuery = %q", got)
	}
}
