# spotGO — Etapa 3: tipo Song, parse de URL e cliente HTTP compartilhado

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Criar o modelo `Song`, o parser de links do Spotify e o pacote `internal/fetch`, que é o cliente HTTP usado por embed, Deezer e Spotify: retry que respeita `Retry-After`, limitador por host e cache em disco.

**Architecture:** `internal/song` não depende de nada. `internal/fetch` só depende da stdlib. Relógio, espera e diretório de cache são injetados para os testes rodarem sem esperar de verdade e sem rede (`httptest`).

**Tech Stack:** Go 1.22, só a stdlib (o limitador é escrito à mão, sem `x/time/rate`).

**Spec:** `docs/superpowers/specs/2026-11-21-spotgo-design.md`

## Global Constraints

- `go 1.22`, nenhuma dependência nova.
- Comentários só onde forem de extrema importância (preferência do usuário para o projeto todo).
- O Claude não faz commit nem push. Para após cada tarefa com a mensagem sugerida.
- Limites por host (padrão do `New`): `api.deezer.com` 1 req/100 ms (50 a cada 5 s), `open.spotify.com` 1 req/250 ms, `api.spotify.com` 1 req/100 ms. Host sem configuração → sem limite.
- `Retry-After` acima de `MaxWait` (padrão 60 s) → **não espera**: retorna `*RateLimitError` na hora (o spotDL travava 24 h nesse caso).
- Corpo de resposta limitado a `MaxBody` (padrão 16 MiB).

## Review Focus

1. **`Retry-After: 86400`** (o sintoma do spotDL) → falha imediata com `*RateLimitError{RetryAfter: 24h}` e uma mensagem legível, sem dormir. Teste na Task 2.
2. **Ctrl+C durante um backoff ou uma espera do limitador** → retorna `ctx.Err()` na hora. Testes nas Tasks 2 e 3.
3. **Arquivo de cache corrompido ou truncado** (queda no meio da escrita, disco cheio) → tratado como miss e regravado. Nunca devolve lixo. Teste na Task 4.
4. **Link do Spotify com variações reais**: `?si=...`, `/intl-pt/`, `spotify:track:...`, `/embed/...`, espaços nas pontas. Tipos não suportados (`episode`, `show`) → erro que diz o tipo. Teste na Task 1.
5. **Várias goroutines no mesmo host** → o limitador espaça todas juntas, sem data race (`-race`). Teste na Task 3.

---

### Task 1: `internal/song`

**Files:** Create `internal/song/song.go`, `internal/song/url.go`. Test `internal/song/url_test.go`, `internal/song/song_test.go`

**Interfaces — Produces:**
- `type Song struct { Title string; Artists []string; Album string; AlbumArtist string; TrackNumber, TrackCount, DiscNumber int; Duration time.Duration; ReleaseDate string; ISRC string; CoverURL string; Explicit bool; SpotifyID string }`
- `func (s Song) MainArtist() string` (primeiro artista, ou `""`)
- `func (s Song) SpotifyURL() string` → `https://open.spotify.com/track/<id>`, ou `""` se não houver ID
- `type Kind string` com `KindTrack="track"`, `KindAlbum="album"`, `KindPlaylist="playlist"`, `KindArtist="artist"`
- `type Ref struct { Kind Kind; ID string }`
- `func ParseURL(raw string) (Ref, error)`: aceita `https://open.spotify.com/[intl-xx/][embed/]<kind>/<id>[?...]` e `spotify:<kind>:<id>`. O ID tem 22 caracteres base62.

- [ ] **Step 1: Testes que falham**
  - `TestParseURL` (tabela, válidos):
    - `https://open.spotify.com/track/4HCOHRCSQelz2GPX8iTamO?si=abc`
    - `https://open.spotify.com/intl-pt/album/0ETFjACtuP2ADo6LFhL6HN`
    - `https://open.spotify.com/embed/playlist/37i9dQZF1DXcBWIGoYBM5M`
    - `spotify:artist:3WrFJ7ztbogyGnTHbHJFl2`
    - `"  https://open.spotify.com/track/4HCOHRCSQelz2GPX8iTamO \n"`
  - `TestParseURLErrors`: `https://open.spotify.com/episode/<22>` → erro contém `episode`; `https://example.com/track/<22>` → erro; ID com 21 caracteres → erro; `""` → erro.
  - `TestMainArtistAndURL`: `Song{Artists: []string{"A","B"}, SpotifyID: "x"}` → `"A"` e `https://open.spotify.com/track/x`; `Song{}` → `""` e `""`.
- [ ] **Step 2:** `go test ./internal/song/` → FAIL (não compila)
- [ ] **Step 3:** Implementar (`net/url` para os links https, `strings.Split` para `spotify:`)
- [ ] **Step 4:** `go test -race ./internal/song/` → PASS
- [ ] **Step 5: PARAR para commit:** `feat(song): tipo Song e parse de links do Spotify`

---

### Task 2: `internal/fetch`: Get com retry

**Files:** Create `internal/fetch/fetch.go`, `internal/fetch/errors.go`. Test `internal/fetch/fetch_test.go`

**Interfaces — Produces:**
- `type Request struct { URL string; Header http.Header; CacheTTL time.Duration }` (`CacheTTL` 0 = não usa cache; usado na Task 4)
- `type Options struct { HTTP *http.Client; MaxRetries int; MaxWait time.Duration; MaxBody int64; UserAgent string; CacheDir string; Limits map[string]time.Duration }`. Zeros viram os padrões: `MaxRetries=3`, `MaxWait=60s`, `MaxBody=16<<20`, `HTTP` com `Timeout: 30s`.
- `func New(o Options) *Client`
- `func (c *Client) Get(ctx context.Context, r Request) ([]byte, error)`
- `type StatusError struct { Code int; URL string; Body string }` (`Body` limitado a 512 bytes), com `Error()`
- `type RateLimitError struct { RetryAfter time.Duration; URL string }`, com `Error()` → `"limite de requisições em <host>; tente de novo em <duração arredondada>"`
- Campos não exportados injetáveis nos testes: `sleep func(ctx context.Context, d time.Duration) error` e `now func() time.Time`.
- Política: 429 → `Retry-After` (segundos **ou** data HTTP). Sem o header, usa o backoff. `> MaxWait` → `*RateLimitError` sem dormir. 5xx e erro de transporte → backoff `500ms·2^n` com jitter de ±20%. Outros 4xx → `*StatusError` sem retry. Esgotou as tentativas → o último erro (`*StatusError`, `*RateLimitError` ou o de transporte).

- [ ] **Step 1: Testes que falham** (`httptest`, `sleep` falso que registra as durações):
  - `TestGetOK`: corpo devolvido, `User-Agent` enviado, header do `Request` repassado.
  - `TestRetryAfterSecondsThenOK`: 429 + `Retry-After: 2` e depois 200 → OK; `sleep` chamado com 2 s; 2 chamadas.
  - `TestRetryAfterHTTPDate`: `Retry-After` = `now+5s` em `http.TimeFormat` → `sleep` ≈ 5 s (com `now` injetado).
  - `TestRetryAfterTooLongFailsFast`: `Retry-After: 86400` → `errors.As(err, &rl)`, `rl.RetryAfter == 24h`, 1 chamada, `sleep` não chamado, a mensagem contém `24h`.
  - `TestServerErrorBackoffThenGiveUp`: sempre 503, `MaxRetries=3` → 4 chamadas, `*StatusError{Code:503}`, os 3 sleeps crescem (cada um ≥ 0,8 × o esperado).
  - `TestNotFoundNoRetry`: 404 → 1 chamada, `*StatusError{Code:404}`.
  - `TestContextCanceledDuringBackoff`: 503 + `sleep` real e ctx cancelado → `errors.Is(err, context.Canceled)`, em menos de 1 s.
  - `TestBodyTooLarge`: `MaxBody=10` e um corpo de 11 bytes → erro que contém `"resposta maior que o limite"`.
- [ ] **Step 2:** `go test ./internal/fetch/` → FAIL
- [ ] **Step 3:** Implementar (`io.LimitReader(body, MaxBody+1)` para detectar o excesso; `defer resp.Body.Close()`)
- [ ] **Step 4:** `go test -race ./internal/fetch/` → PASS
- [ ] **Step 5: PARAR para commit:** `feat(fetch): cliente HTTP com retry e Retry-After`

---

### Task 3: `internal/fetch`: limitador por host

**Files:** Create `internal/fetch/limiter.go`. Modify `fetch.go` (chamar `wait` antes de cada tentativa). Test `internal/fetch/limiter_test.go`

**Interfaces — Produces:**
- `type limiter struct` (`sync.Mutex` + próximo horário livre + intervalo)
- `func (l *limiter) wait(ctx context.Context, now func() time.Time, sleep func(context.Context, time.Duration) error) error`: reserva o próximo horário (`next = max(now, next) + interval`) **dentro** do lock e dorme **fora** do lock.
- `New` copia `Options.Limits` e, quando `Limits == nil`, usa os padrões das Global Constraints.

- [ ] **Step 1: Testes que falham**
  - `TestLimiterSpacing`: com intervalo 100 ms e relógio falso parado, 3 `wait` seguidos → sleeps de 0, 100 e 200 ms.
  - `TestLimiterConcurrent`: 20 goroutines com relógio real e intervalo 5 ms → tempo total ≥ 95 ms, e `-race` limpo.
  - `TestLimiterCanceled`: ctx cancelado → `wait` devolve `context.Canceled`.
  - `TestGetUsesHostLimit`: `Limits{<host do httptest>: 50ms}` e 2 `Get` → o segundo chama `sleep(≈50ms)`.
- [ ] **Step 2:** FAIL. **Step 3:** implementar. **Step 4:** `go test -race ./internal/fetch/` → PASS
- [ ] **Step 5: PARAR para commit:** `feat(fetch): limitador de requisições por host`

---

### Task 4: `internal/fetch`: cache em disco

**Files:** Create `internal/fetch/cache.go`. Modify `fetch.go`. Test `internal/fetch/cache_test.go`

**Interfaces — Produces:**
- Com `Request.CacheTTL > 0` e `Options.CacheDir != ""`: a chave é `sha256(URL)` em hex, e o arquivo fica em `<CacheDir>/<2 primeiros hex>/<hex>`. Formato: 8 bytes big-endian com o `UnixNano` da gravação, seguidos do corpo. A escrita é atômica (`CreateTemp` + `Rename`). A pasta é criada com `0700` e o arquivo com `0600`.
- Hit: idade `< TTL` (pelo `now` injetado) → devolve sem rede e **sem passar pelo limitador**. Miss ou expirado → `Get` normal, e só uma resposta 200 é gravada.
- Arquivo com menos de 8 bytes ou ilegível → trata como miss (regrava depois).
- `func DefaultCacheDir() (string, error)` → `os.UserCacheDir()/spotgo/http`

- [ ] **Step 1: Testes que falham**
  - `TestCacheHit`: duas chamadas com TTL de 1 h → o servidor recebe 1 chamada, e os dois corpos são iguais.
  - `TestCacheExpired`: `now` avança 2 h com TTL de 1 h → 2 chamadas.
  - `TestCacheSkipsErrors`: 404 e depois 200 → o 404 não é cacheado (2 chamadas).
  - `TestCacheCorruptFileIsMiss`: gravar 3 bytes no caminho da chave → `Get` vai à rede, devolve o corpo certo e regrava.
  - `TestNoCacheWhenTTLZero`: TTL 0 → 2 chamadas, e nenhum arquivo é criado.
- [ ] **Step 2:** FAIL. **Step 3:** implementar. **Step 4:** `go vet ./... && go test -race ./...` → PASS
- [ ] **Step 5: PARAR para commit:** `feat(fetch): cache de respostas em disco`
