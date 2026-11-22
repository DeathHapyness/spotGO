# spotGO — Etapa 4: leitura da página pública de embed

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Pacote `internal/embed` que transforma `open.spotify.com/embed/{track|album|playlist|artist}/{id}` em `song.Song` e `embed.Collection`, sem credenciais.

**Architecture:** Duas camadas. `parse.go` contém funções puras sobre `[]byte` (HTML → entidade → modelos), testadas com fixtures reais reduzidas. `client.go` busca a página via `fetch.Client` (limitador + cache) e chama o parser. Os testes do client servem as fixtures por `httptest`.

**Tech Stack:** Go 1.22, só a stdlib (`regexp` ou `bytes.Index` para achar o `__NEXT_DATA__`, `encoding/json`).

**Spec:** `docs/superpowers/specs/2026-11-21-spotgo-design.md` (seção "Modo público: embed + Deezer")

## Global Constraints

- Comentários só onde forem de extrema importância.
- O Claude não faz commit nem push. Para após cada tarefa.
- O caminho do JSON é `props.pageProps.state.data.entity`, dentro de `<script id="__NEXT_DATA__" type="application/json">`.
- Os artistas em `subtitle` são separados por `", "` (vírgula + NBSP). **Nunca** separar por `","` simples.
- Capa: maior `maxWidth` de `entity.visualIdentity.image[]`.
- TTL do cache: faixa e álbum 30 dias, artista 1 dia, playlist 10 min.
- Base da URL configurável (`Client.base`), com padrão `https://open.spotify.com/embed`, para que os testes apontem para o `httptest`.
- Fixtures: copiar de `/tmp/claude-1000/-home-rique/c1132ad9-1afc-4403-b7ea-4e93fe111a65/scratchpad/embed/fixtures/{track,album,playlist,artist}.html` para `internal/embed/testdata/`. São páginas reais de 2026-11-22, com o `trackList` cortado em 3 faixas.

## Review Focus

1. **Artista com vírgula no nome** (`"Tyler, The Creator, Kali Uchis"`) → `["Tyler, The Creator", "Kali Uchis"]`. Teste na Task 1.
2. **Playlist com exatamente 100 faixas** → `Truncated == true`. Com 99 → `false`. Teste na Task 1 (página sintética gerada no teste).
3. **HTML sem `__NEXT_DATA__`, JSON quebrado ou sem `entity`** (o Spotify mudou a página, captcha, página de erro com status 200) → `ErrFormatChanged`, sem panic. Teste na Task 1.
4. **ID inexistente** → HTTP 404 → erro que satisfaz `errors.Is(err, ErrNotFound)` e cita o link. Teste na Task 2.
5. **Itens que não são faixa** (`entityType: "episode"`, `uri` `spotify:local:...`, `isPlayable: false`) → pulados e contados em `Collection.Skipped`. Teste na Task 1.

---

### Task 1: Parser (`parse.go`)

**Files:** Create `internal/embed/parse.go`, `internal/embed/testdata/*.html` (copiar as fixtures). Test `internal/embed/parse_test.go`

**Interfaces — Produces:**
- `var ErrFormatChanged = errors.New("o formato da página de embed do Spotify mudou; atualize o spotgo")`
- `const PlaylistLimit = 100`
- `type Collection struct { Kind song.Kind; ID, Name, Artist, CoverURL string; Songs []song.Song; Truncated bool; Skipped int }`
- `func parseTrack(page []byte) (song.Song, error)`: `Title=name`, `Artists` de `entity.artists[].name`, `Duration` (ms → `time.Duration`), `Explicit`, `ReleaseDate` = os 10 primeiros caracteres de `releaseDate.isoString` (ou `""`), `CoverURL`, `SpotifyID` (do `uri`).
- `func parseCollection(page []byte, kind song.Kind) (Collection, error)`: `Name=entity.name`, `Artist=entity.subtitle` (álbum/artista), `CoverURL`, e as `Songs` do `trackList`. Cada faixa recebe `Title`, `Artists=splitArtists(subtitle)`, `Duration`, `Explicit` e `SpotifyID`. **Só no álbum**, cada faixa recebe também `Album=Name`, `AlbumArtist=Artist`, `CoverURL` do álbum, `TrackNumber=i+1` (contando só as aceitas) e `TrackCount=len(Songs)`. **Só na playlist**, `Truncated = len(trackList) >= PlaylistLimit`. `ID` sai do `entity.uri`.
- `func splitArtists(s string) []string`
- `func bestImage(imgs []image) string`

- [ ] **Step 1: Testes que falham**
  - `TestParseTrackFixture` (`testdata/track.html`): `Title` começa com `"Flower In A Hurricane"`; `Artists[0]=="Justice"`; `len(Artists)==5`; `Duration==224877ms`; `ReleaseDate=="2026-10-15"`; `SpotifyID=="4HCOHRCSQelz2GPX8iTamO"`; `CoverURL` contém `ab67616d0000b273` (o de 640 px).
  - `TestParseAlbumFixture`: `Name=="Abbey Road (Remastered 2009)"`; `Artist=="The Beatles"`; 3 faixas; `Songs[0].Title=="Come Together - Remastered 2009"`, `TrackNumber==1`, `TrackCount==3`, `Album==Name`, `AlbumArtist=="The Beatles"`, `Duration==259946ms`.
  - `TestParsePlaylistFixture`: `Name=="Today’s Top Hits"` (apóstrofo tipográfico), 3 faixas, `Songs[0].Artists` com 5 nomes, `Album==""`, `Truncated==false`.
  - `TestParseArtistFixture`: `Name=="The Beatles"`, `len(Songs)==3`.
  - `TestSplitArtists` (tabela): `"A, B"` → `[A B]`; `"Tyler, The Creator, Kali Uchis"` → `[Tyler, The Creator; Kali Uchis]`; `"Solo"` → `[Solo]`; `""` → `nil`; `" A ,  B "` → `[A B]`.
  - `TestPlaylistTruncated`: o helper `syntheticPage(t, kind, n)` gera uma página com `n` faixas válidas → `n=100` dá `Truncated` e `n=99` não dá.
  - `TestSkipsNonTracks`: uma página sintética com 1 faixa válida, 1 `entityType:"episode"`, 1 `uri:"spotify:local:..."` e 1 `isPlayable:false` → `len(Songs)==1`, `Skipped==3`, e (no álbum) `TrackNumber==1`.
  - `TestFormatChanged` (tabela): `""`, `"<html>sem script</html>"`, um script com `{quebrado`, e um JSON válido sem `entity` → `errors.Is(err, ErrFormatChanged)`.
- [ ] **Step 2:** `go test ./internal/embed/` → FAIL
- [ ] **Step 3:** Implementar. As structs JSON são não exportadas e espelham só os campos usados. Use `bytes.Index` pelo marcador de abertura e por `</script>`.
- [ ] **Step 4:** `go test -race ./internal/embed/` → PASS
- [ ] **Step 5: PARAR para commit:** `feat(embed): parser da página pública de embed`

---

### Task 2: Client (`client.go`)

**Files:** Create `internal/embed/client.go`. Test `internal/embed/client_test.go`

**Interfaces:**
- Consumes: `fetch.Client.Get`, `fetch.Request`, `*fetch.StatusError` (Etapa 3); `parseTrack` e `parseCollection` (Task 1).
- Produces:
  - `var ErrNotFound = errors.New("não encontrado no Spotify")`
  - `type Client struct { f *fetch.Client; base string }`
  - `func New(f *fetch.Client) *Client` (`base` = `https://open.spotify.com/embed`)
  - `func (c *Client) Track(ctx context.Context, id string) (song.Song, error)`
  - `func (c *Client) Album(ctx context.Context, id string) (Collection, error)`
  - `func (c *Client) Playlist(ctx context.Context, id string) (Collection, error)`
  - `func (c *Client) Artist(ctx context.Context, id string) (Collection, error)`
  - Um 404 vira `fmt.Errorf("%w: %s", ErrNotFound, <url pública open.spotify.com/<kind>/<id>>)`. Outros erros recebem wrap com o tipo e o ID.

- [ ] **Step 1: Testes que falham** (um `httptest` que serve `testdata/<kind>.html` em `/<kind>/<id>` e 404 para o ID `"0000000000000000000000"`):
  - `TestClientTrack`, `TestClientAlbum`, `TestClientPlaylist`, `TestClientArtist`: conferem o caminho pedido (`/track/<id>` etc.) e um campo de cada resultado.
  - `TestClientNotFound`: o ID de 404 → `errors.Is(err, ErrNotFound)`, e a mensagem contém `open.spotify.com/track/0000000000000000000000`.
  - `TestClientUsesCache`: `fetch` com `CacheDir: t.TempDir()` e 2 `Album` → o servidor recebe 1 chamada.
- [ ] **Step 2:** FAIL. **Step 3:** implementar. **Step 4:** `go vet ./... && go test -race ./...` → PASS
- [ ] **Step 5: PARAR para commit:** `feat(embed): cliente para faixa, álbum, playlist e artista`
