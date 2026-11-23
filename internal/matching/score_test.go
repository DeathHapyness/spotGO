package matching

import (
	"math"
	"testing"
	"time"

	"github.com/DeathHapyness/spotGO/internal/song"
)

// createSongAndResult é o helper create_song_and_result do spotDL.
func createSongAndResult(name string, artists []string, resultName string, resultArtists []string, verified bool) (song.Song, Result) {
	return song.Song{Title: name, Artists: artists, Album: name, Duration: 107 * time.Second},
		Result{
			Source: "youtube-music", URL: "https://music.youtube.com/watch?v=0X93DSg2ZQg", ID: "0X93DSg2ZQg",
			Verified: verified, Name: resultName, Duration: 108 * time.Second,
			Author: resultArtists[0], Artists: resultArtists,
		}
}

func approved(s song.Song, r Result) bool {
	return len(Order(s, []Result{r})) == 1
}

// Casos de test_order_results_single_result_artist (spotDL).
func TestOrderSingleResultArtist(t *testing.T) {
	cases := []struct {
		name          string
		artists       []string
		resultName    string
		resultArtists []string
		verified      bool
		want          bool
	}{
		{"Wasted", []string{"LUCØ", "Alice Gray"}, "Wasted (feat. Alice Gray)", []string{"LUCØ"}, true, true},
		{"U Da One", []string{"Velmar", "Misdemeanor"}, "U Da One", []string{"Velmar und Misdemeanor"}, true, true},
		{"Under Pressure", []string{"Queen", "David Bowie"}, "Under Pressure", []string{"Queens of the Stone Age"}, true, false},
		{"Too Good", []string{"Drake", "Rihanna"}, "Too Good (feat. Rihanna)", []string{"Drake Bell"}, true, false},
		{"Song Title", []string{"Alpha", "Bravo", "Charlie", "Delta"}, "Alpha/Bravo - Song Title (feat. Charlie)", []string{"Alpha"}, false, true},
	}
	for _, c := range cases {
		s, r := createSongAndResult(c.name, c.artists, c.resultName, c.resultArtists, c.verified)
		if got := approved(s, r); got != c.want {
			t.Errorf("%q × %q %v: aprovado = %v, quero %v", c.name, c.resultName, c.resultArtists, got, c.want)
		}
	}
}

func TestAlbumMatchWithoutSongAlbum(t *testing.T) {
	if got := albumMatch(song.Song{}, Result{Album: "Album"}); got != 0 {
		t.Fatalf("albumMatch = %v, quero 0", got)
	}
	if got := albumMatch(song.Song{Album: "Abbey Road"}, Result{Album: "Abbey Road"}); got != 100 {
		t.Fatalf("albumMatch igual = %v, quero 100", got)
	}
}

func TestTimeMatch(t *testing.T) {
	s := song.Song{Duration: 200 * time.Second}
	if got := timeMatch(s, Result{Duration: 200 * time.Second}); got != 100 {
		t.Errorf("mesma duração = %v", got)
	}
	if got := timeMatch(s, Result{Duration: 210 * time.Second}); !near(got, 100*math.Exp(-1)) {
		t.Errorf("10 s = %v, quero %v", got, 100*math.Exp(-1))
	}
}

func comeTogether() (song.Song, []Result) {
	s := song.Song{
		Title: "Come Together - Remastered 2009", Artists: []string{"The Beatles"},
		Album: "Abbey Road (Remastered 2009)", Duration: 259946 * time.Millisecond,
	}
	beatles := []string{"The Beatles"}
	return s, []Result{
		{ID: "A", Verified: true, Name: "Come Together (Remastered 2009)", Artists: beatles, Duration: 260 * time.Second, Album: "Abbey Road (Remastered 2009)"},
		{ID: "B", Verified: true, Name: "Come Together (Live)", Artists: beatles, Duration: 259 * time.Second},
		{ID: "C", Name: "Come Together (Karaoke Version)", Author: "Sing King", Artists: []string{"Sing King"}, Duration: 259 * time.Second},
		{ID: "D", Verified: true, Name: "Come Together", Artists: beatles, Duration: 8 * time.Minute},
	}
}

func TestOrderComeTogether(t *testing.T) {
	s, results := comeTogether()
	scored := Order(s, results)
	scores := map[string]float64{}
	for _, sc := range scored {
		scores[sc.Result.ID] = sc.Score
	}
	if _, ok := scores["A"]; !ok {
		t.Fatalf("estúdio reprovado: %v", scores)
	}
	if b, ok := scores["B"]; ok && b >= scores["A"] {
		t.Errorf("ao vivo (%v) >= estúdio (%v)", b, scores["A"])
	}
	for _, id := range []string{"C", "D"} {
		if _, ok := scores[id]; ok {
			t.Errorf("%s aprovado: %v", id, scores)
		}
	}
	best, ok := Best(scored)
	if !ok || best.Result.ID != "A" {
		t.Fatalf("Best = %+v, %v; quero A", best, ok)
	}
}

func TestOrderPlainTitleStudio(t *testing.T) {
	s, _ := comeTogether()
	r := Result{Verified: true, Name: "Come Together", Artists: []string{"The Beatles"}, Duration: 259 * time.Second, Album: "Abbey Road"}
	if !approved(s, r) {
		t.Fatalf("\"Come Together\" sem versão reprovado para %q", s.Title)
	}
}

func TestOrderExplicitMismatch(t *testing.T) {
	// Álbum diferente num resultado verificado puxa a nota para ≤ 85; só nessa
	// faixa o spotDL soma a duração e aplica a penalidade de explícito.
	s := song.Song{Title: "Song", Artists: []string{"Artist"}, Album: "Song", Duration: 200 * time.Second}
	yes, no := true, false
	base := Result{Verified: true, Name: "Song", Artists: []string{"Artist"}, Album: "Different Album", Duration: 200 * time.Second}
	clean, explicit := base, base
	clean.Explicit, explicit.Explicit = &no, &yes
	s.Explicit = true
	got := Order(s, []Result{clean, explicit})
	if len(got) != 2 || !near(got[1].Score-got[0].Score, 5) {
		t.Fatalf("Order = %+v, quero explícito 5 pontos acima", got)
	}
	if got[1].Score > 85 {
		t.Fatalf("nota %v: o caso não exercita o bloco <= 85", got[1].Score)
	}
}

func TestOrderExplicitIgnoredWhenHigh(t *testing.T) {
	s := song.Song{Title: "Song", Artists: []string{"Artist"}, Explicit: true, Duration: 200 * time.Second}
	no := false
	got := Order(s, []Result{{Name: "Song Here", Artists: []string{"Artist"}, Duration: 200 * time.Second, Explicit: &no}})
	if len(got) != 1 || got[0].Score <= 85 {
		t.Fatalf("Order = %+v: nota > 85 não leva penalidade de explícito (como no spotDL)", got)
	}
}

func TestOrderEmptyInputs(t *testing.T) {
	cases := []struct {
		s song.Song
		r Result
	}{
		{song.Song{Title: "X", Duration: time.Minute}, Result{Name: "X", Artists: []string{"A"}, Duration: time.Minute}},
		{song.Song{Title: "X", Artists: []string{"A"}, Duration: time.Minute}, Result{Name: "X", Duration: time.Minute}},
		{song.Song{Title: "🔥", Artists: []string{"LUCØ", "🔥"}, Duration: time.Minute}, Result{Name: "🔥", Artists: []string{"LUCØ"}, Duration: time.Minute, Verified: true}},
		{song.Song{}, Result{}},
	}
	for _, c := range cases {
		for _, sc := range Order(c.s, []Result{c.r}) {
			if math.IsNaN(sc.Score) || math.IsInf(sc.Score, 0) {
				t.Errorf("Order(%+v, %+v) = %v", c.s, c.r, sc.Score)
			}
		}
	}
}

func TestBestViews(t *testing.T) {
	sc := []Scored{
		{Result{ID: "a", Views: 100}, 90},
		{Result{ID: "b", Views: 1000}, 88},
		{Result{ID: "c", Views: 5000}, 70},
	}
	if got, _ := Best(sc); got.Result.ID != "b" || got.Score != 100 {
		t.Errorf("Best = %s %v, quero b com 100", got.Result.ID, got.Score)
	}
	for i := range sc {
		sc[i].Result.Views = 0
	}
	if got, _ := Best(sc); got.Result.ID != "a" {
		t.Errorf("sem views: Best = %s, quero a", got.Result.ID)
	}
	if _, ok := Best(nil); ok {
		t.Error("Best(nil) ok=true")
	}
	if got, _ := Best([]Scored{{Result{ID: "x"}, 50}, {Result{ID: "y"}, 95}}); got.Result.ID != "y" {
		t.Errorf("fora de ordem: Best = %s, quero y", got.Result.ID)
	}
}

func TestBestISRC(t *testing.T) {
	sc := []Scored{
		{Result{ID: "isrc", ISRCSearch: true, Views: 10}, 85},
		{Result{ID: "outro", Views: 99999}, 84},
	}
	if got, _ := Best(sc); got.Result.ID != "isrc" {
		t.Fatalf("Best = %s, quero isrc", got.Result.ID)
	}
}

// O spotDL buscava as views que faltavam; aqui 0 = desconhecido. Com alguma
// desconhecida, não dá para comparar: fica o de maior nota.
func TestBestViewsPartiallyUnknown(t *testing.T) {
	sc := []Scored{
		{Result{ID: "a"}, 90},
		{Result{ID: "b", Views: 1000}, 88},
	}
	if got, _ := Best(sc); got.Result.ID != "a" || got.Score != 90 {
		t.Fatalf("Best = %s %v, quero a com 90 (views de a desconhecidas)", got.Result.ID, got.Score)
	}
}
