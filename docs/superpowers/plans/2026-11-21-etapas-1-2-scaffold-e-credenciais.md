# spotGO — Etapas 1 e 2: Esqueleto Go e Credenciais Interativas

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Trocar o código Python por um esqueleto Go com o binário `spotgo` e implementar a resolução de credenciais do Spotify com o assistente interativo e o comando `spotgo auth`.

**Architecture:** `cmd/spotgo` contém só a camada de CLI (cobra). A lógica fica em `internal/config` (arquivo de config, resolução de credenciais, assistente) e `internal/spotify` (por enquanto só o pedido de token, usado para validar as credenciais). O assistente recebe `io.Reader`/`io.Writer` e funções injetadas, então pode ser testado sem terminal e sem rede.

**Tech Stack:** Go 1.22, `github.com/spf13/cobra@v1.8.1`, `golang.org/x/term@v0.25.0`, `net/http/httptest` para os testes.

**Spec:** `docs/superpowers/specs/2026-11-21-spotgo-design.md`

## Global Constraints

- `go 1.22` no `go.mod`. As versões das dependências ficam fixas, como acima: versões mais novas do `x/term` exigem Go 1.23, e o `GOTOOLCHAIN=auto` trocaria o toolchain sozinho.
- Módulo: `github.com/DeathHapyness/spotGO`. Binário: `spotgo`, em `cmd/spotgo/`.
- Nenhum `client_id` embutido no código.
- Config em `os.UserConfigDir()/spotgo/config.json`, arquivo `0600` e pasta `0700`.
- Ordem de resolução: flags → env (`SPOTGO_CLIENT_ID`/`SPOTGO_CLIENT_SECRET`) → arquivo → assistente (só com TTY) → erro.
- Endpoint de token: `POST https://accounts.spotify.com/api/token`, `grant_type=client_credentials`, Basic Auth.
- **O Claude não roda `git commit` nem `git push`.** Em cada passo "Commit", o Claude para, mostra os arquivos e a mensagem sugerida, e espera o usuário commitar.
- O usuário está aprendendo Go: cada tarefa termina com uma explicação curta das decisões idiomáticas.

## Review Focus

1. **Credenciais coladas com espaço ou quebra de linha** (comum ao copiar do dashboard) → aparar com `strings.TrimSpace` antes de validar e salvar. Teste na Task 3.
2. **`config.json` corrompido ou editado à mão** → erro que cita o caminho e sugere `spotgo auth`, sem panic. Teste na Task 2.
3. **Só metade da credencial numa fonte** (só `--client-id`, ou só `SPOTGO_CLIENT_ID`) → erro dizendo que as duas são obrigatórias. Nunca misturar o ID de uma fonte com o secret de outra. Teste na Task 2.
4. **Stdin não interativo e sem credenciais** (cron, pipe) → `ErrNoCredentials` com instruções, sem travar esperando entrada. Teste na Task 4.
5. **Falha de rede na validação ≠ credencial inválida** → o 400/401 `invalid_client` pede as credenciais de novo. Erro de rede aborta sem salvar e com mensagem diferente. Testes nas Tasks 3 e 4.

---

### Task 1: Remover o Python e criar o esqueleto Go

**Files:**
- Delete: `spotdl/`, `tests/`, `scripts/`, `docs/*.md` (manter `docs/superpowers/`), `pyproject.toml`, `uv.lock`, `.pylintrc`, `mkdocs.yml`, `.readthedocs.yaml`, `Dockerfile`, `docker-compose.yml`, `.dockerignore`, `.vscode/`, `.github/` inteiro (workflows de Python, FUNDING e templates são do spotDL)
- Modify: `LICENSE`: manter o texto MIT e o `Copyright (c) 2021 spotDL Developers`, e adicionar a linha `Copyright (c) 2026 DeathHapyness` acima dele
- Modify: `.editorconfig`: remover `[*.py]`, adicionar `[*.go]` com `indent_style = tab`
- Modify: `.gitattributes`: trocar `*.py text diff=python` por `*.go text diff=golang`
- Rewrite: `.gitignore` (binário `/spotgo`, `/dist/`, `*.test`, `*.out`, `coverage.*`, `.idea/`, `.vscode/`)
- Rewrite: `README.md` (o que é, aviso de não afiliação com o Spotify, requisitos, `go install`, como criar o app no Spotify, uso básico, créditos ao spotDL)
- Create: `go.mod`, `cmd/spotgo/main.go`, `cmd/spotgo/root.go`, `cmd/spotgo/root_test.go`
- Create: `.github/workflows/go.yml` (push/PR: `go vet ./...`, `go test -race ./...`, Go 1.22)

**Interfaces:**
- Produces: `func newRootCmd() *cobra.Command`, com `Use: "spotgo"`, a flag persistente `--client-id`/`--client-secret` (strings) e `Version` vindo de `var version = "dev"` (sobrescrito via `-ldflags "-X main.version=..."`). `main()` só chama `newRootCmd().ExecuteContext(ctx)`, com `ctx` vindo de `signal.NotifyContext(..., os.Interrupt)`, e sai com código 1 em erro.

- [ ] **Step 1: Apagar os arquivos Python listados acima** (`git rm -r`; o usuário vê tudo no `git status`)
- [ ] **Step 2: `go mod init github.com/DeathHapyness/spotGO`, editar a diretiva para `go 1.22` e rodar `go get github.com/spf13/cobra@v1.8.1`**
- [ ] **Step 3: Escrever o teste que falha** em `cmd/spotgo/root_test.go`:

```go
func TestRootVersion(t *testing.T) {
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil { t.Fatal(err) }
	if !strings.Contains(out.String(), "spotgo version dev") { t.Fatalf("got %q", out.String()) }
}
```

- [ ] **Step 4: `go test ./cmd/...`** → FAIL (`newRootCmd` não definido)
- [ ] **Step 5: Implementar `main.go` e `root.go`**
- [ ] **Step 6: `go vet ./... && go test ./... && go build -o spotgo ./cmd/spotgo && ./spotgo --help`** → PASS, e a ajuda lista as flags
- [ ] **Step 7: Arquivos auxiliares** (LICENSE, `.editorconfig`, `.gitattributes`, `.gitignore`, README, CI)
- [ ] **Step 8: PARAR para o usuário commitar.** Mensagem sugerida:

```
chore: remove código Python e cria esqueleto Go do spotgo
```

---

### Task 2: `internal/config`: arquivo e resolução de credenciais

**Files:**
- Create: `internal/config/config.go`, `internal/config/resolve.go`
- Test: `internal/config/config_test.go`, `internal/config/resolve_test.go`

**Interfaces:**
- Produces:
  - `type Credentials struct { ClientID string \`json:"client_id"\`; ClientSecret string \`json:"client_secret"\` }`, com o método `(c Credentials) Complete() bool` (os dois não vazios).
  - `type Config struct { Spotify Credentials \`json:"spotify"\` }`
  - `func DefaultPath() (string, error)`: `os.UserConfigDir()/spotgo/config.json`
  - `func Load(path string) (Config, error)`: arquivo inexistente → `Config{}`, nil. JSON inválido → erro que contém o `path` e a palavra `spotgo auth`.
  - `func Save(path string, c Config) error`: cria a pasta com `0700`, escreve em `path+".tmp"` com `0600`, `fsync` e `os.Rename` (escrita atômica).
  - `func Delete(path string) error`: arquivo inexistente não é erro.
  - `type Source string`, com as constantes `SourceFlags = "flags"`, `SourceEnv = "env"`, `SourceFile = "file"`
  - `var ErrNoCredentials = errors.New(...)`, com uma mensagem que inclui `https://developer.spotify.com/dashboard` e `spotgo auth`.
  - `func Resolve(flags Credentials, getenv func(string) string, path string) (Credentials, Source, error)`: aplica `TrimSpace` em tudo. Uma fonte com só um dos dois campos → erro `"--client-id e --client-secret devem ser usados juntos"` ou o equivalente com o nome das variáveis de ambiente. Nada encontrado → `ErrNoCredentials`.

- [ ] **Step 1: Escrever os testes que falham**
  - `TestSaveLoadRoundTrip`: `Save` e depois `Load` em `t.TempDir()` retornam o mesmo valor. `os.Stat` mostra `0600` no arquivo e `0700` na pasta.
  - `TestLoadMissingFile`: retorna `Config{}`, nil.
  - `TestLoadCorrupt`: com o arquivo `{nope`, o erro contém o path e `spotgo auth`.
  - `TestResolve`: teste de tabela com os casos:
    - flags completas ganham de env e de arquivo → `SourceFlags`
    - env completa ganha do arquivo → `SourceEnv`
    - só o arquivo → `SourceFile`
    - só `--client-id` → erro que contém `--client-secret`
    - só `SPOTGO_CLIENT_ID` → erro que contém `SPOTGO_CLIENT_SECRET`
    - `"  abc\n"` nas flags → `ClientID == "abc"`
    - nada → `errors.Is(err, ErrNoCredentials)`
- [ ] **Step 2: `go test ./internal/config/`** → FAIL (não compila)
- [ ] **Step 3: Implementar `config.go` e `resolve.go`**
- [ ] **Step 4: `go test -race ./internal/config/`** → PASS
- [ ] **Step 5: PARAR para o usuário commitar.** Mensagem sugerida: `feat(config): arquivo de config e resolução de credenciais`

---

### Task 3: `internal/spotify`: pedido de token (validação)

**Files:**
- Create: `internal/spotify/token.go`
- Test: `internal/spotify/token_test.go`

**Interfaces:**
- Consumes: `config.Credentials`
- Produces:
  - `const TokenURL = "https://accounts.spotify.com/api/token"`
  - `type Token struct { AccessToken string; ExpiresAt time.Time }`
  - `var ErrInvalidCredentials = errors.New("credenciais do Spotify inválidas")`
  - `func RequestToken(ctx context.Context, hc *http.Client, tokenURL string, c config.Credentials) (Token, error)`: Basic Auth com ID:secret, form `grant_type=client_credentials`. Resposta 400/401 → `ErrInvalidCredentials` (com wrap). Qualquer outro status diferente de 200 → erro com o status. Falha de transporte → erro com wrap que **não** é `ErrInvalidCredentials`. `ExpiresAt = now + expires_in`.

- [ ] **Step 1: Escrever os testes que falham** (servidor `httptest`):
  - `TestRequestTokenOK`: o servidor confere `r.BasicAuth()` e `grant_type=client_credentials` e responde `{"access_token":"tok","token_type":"Bearer","expires_in":3600}` → `AccessToken=="tok"`, e `ExpiresAt` fica entre now+3590s e now+3600s.
  - `TestRequestTokenInvalid`: 400 `{"error":"invalid_client"}` → `errors.Is(err, ErrInvalidCredentials)`.
  - `TestRequestTokenNetwork`: servidor fechado (`srv.Close()` antes da chamada) → `err != nil` e `!errors.Is(err, ErrInvalidCredentials)`.
- [ ] **Step 2: `go test ./internal/spotify/`** → FAIL
- [ ] **Step 3: Implementar `token.go`**
- [ ] **Step 4: `go test -race ./internal/spotify/`** → PASS
- [ ] **Step 5: PARAR para o usuário commitar.** Mensagem sugerida: `feat(spotify): pedido de token client credentials`

---

### Task 4: Assistente interativo e garantia de credenciais

**Files:**
- Create: `internal/config/wizard.go`
- Test: `internal/config/wizard_test.go`

**Interfaces:**
- Consumes: `Credentials`, `Resolve`, `Save`, `ErrNoCredentials` (Task 2). A validação é injetada como uma função, para que `config` não importe `spotify`. Quem liga as duas coisas é o `cmd`.
- Produces:
  - `type Validator func(ctx context.Context, c Credentials) error`
  - `type Prompter struct { In *bufio.Reader; Out io.Writer; ReadSecret func() (string, error) }`
  - `func RunWizard(ctx context.Context, p Prompter, validate Validator, isInvalid func(error) bool) (Credentials, error)`: imprime as instruções (URL do dashboard e o passo a passo: criar app → copiar Client ID → "View client secret"), lê o ID pelo `In` e o secret pelo `ReadSecret`, e aplica `TrimSpace`. Valida o formato localmente: 32 caracteres hexadecimais em cada campo; se falhar, pede de novo sem chamar a rede. Chama `validate`: se `isInvalid(err)`, avisa e pede de novo. Outro erro → retorna o erro com wrap, sem salvar. São no máximo 3 tentativas, depois erro.
  - `func Ensure(ctx context.Context, flags Credentials, path string, interactive bool, p Prompter, validate Validator, isInvalid func(error) bool) (Credentials, error)`: `Resolve`. Se der `ErrNoCredentials` e `interactive` → `RunWizard` + `Save` + mensagem "Credenciais salvas em <path>". Se não for interativo → retorna `ErrNoCredentials`.
  - `func Mask(s string) string`: com `len <= 8` → `"********"`. Caso contrário, os 4 primeiros + `"…"` + os 4 últimos.

- [ ] **Step 1: Escrever os testes que falham.** Use `strings.NewReader` como `In`, `ReadSecret` devolvendo itens de um slice e um `validate` falso que conta as chamadas.
  - `TestWizardHappyPath`: o ID `" <32hex>\n"` e o secret `"<32hex>\n"` voltam aparados, e `validate` é chamado 1x.
  - `TestWizardBadFormatRetriesWithoutNetwork`: primeiro ID `"abc"`, depois um válido → `validate` chamado 1x, e a saída contém `32`.
  - `TestWizardInvalidThenValid`: o primeiro `validate` retorna o erro "inválido" → pede de novo, e o segundo passa.
  - `TestWizardNetworkErrorAborts`: `validate` retorna um erro que não é de credencial inválida → `RunWizard` retorna erro, e `validate` foi chamado 1x.
  - `TestWizardGivesUpAfter3`: três inválidos → erro.
  - `TestEnsureNonInteractive`: sem nenhuma fonte e `interactive=false` → `errors.Is(err, ErrNoCredentials)`, e `In` não foi lido.
  - `TestEnsureSavesAfterWizard`: `interactive=true` → o arquivo em `t.TempDir()` passa a existir com as credenciais.
  - `TestMask`: `"abcd1234efgh5678"` → `"abcd…5678"`, `"short"` → `"********"`.
- [ ] **Step 2: `go test ./internal/config/`** → FAIL
- [ ] **Step 3: Implementar `wizard.go`**
- [ ] **Step 4: `go test -race ./internal/config/`** → PASS
- [ ] **Step 5: PARAR para o usuário commitar.** Mensagem sugerida: `feat(config): assistente interativo de credenciais`

---

### Task 5: Comando `spotgo auth`

**Files:**
- Create: `cmd/spotgo/auth.go`, `cmd/spotgo/deps.go` (liga `config.Validator` a `spotify.RequestToken` e cria o `Prompter` real com `term.ReadPassword(int(os.Stdin.Fd()))`; `interactive = term.IsTerminal(int(os.Stdin.Fd()))`)
- Modify: `cmd/spotgo/root.go` (registra `newAuthCmd()`)
- Test: `cmd/spotgo/auth_test.go`

**Interfaces:**
- Consumes: Tasks 2–4.
- Produces: `func newAuthCmd() *cobra.Command`, com estes modos:
  - `spotgo auth` → sempre roda o assistente (mesmo que já existam credenciais) e salva.
  - `spotgo auth --status` → mostra a fonte em uso (`flags`/`env`/`file`), o ID com `Mask` e o caminho do arquivo. Nunca mostra o secret. Sem credenciais → explica como configurar e sai com código 0.
  - `spotgo auth --clear` → `Delete` e confirma.
  - Para testar, o caminho da config pode ser sobrescrito pela variável de ambiente `SPOTGO_CONFIG` (é lida em `deps.go` e também documentada no README).

- [ ] **Step 1: Escrever os testes que falham**
  - `TestAuthStatusMasksSecret`: com `SPOTGO_CONFIG` apontando para um arquivo salvo, a saída contém `Mask(id)` e **não** contém o secret.
  - `TestAuthClear`: o arquivo deixa de existir depois do comando.
- [ ] **Step 2: `go test ./cmd/...`** → FAIL
- [ ] **Step 3: Implementar `auth.go` e `deps.go`**
- [ ] **Step 4: `go vet ./... && go test -race ./...`** → PASS
- [ ] **Step 5: Verificação manual** (o usuário roda no próprio terminal, com credenciais reais): `go build -o spotgo ./cmd/spotgo && ./spotgo auth`, depois `./spotgo auth --status`, depois `ls -l ~/.config/spotgo/config.json` (deve mostrar `-rw-------`), e por fim `echo | ./spotgo auth --status`.
- [ ] **Step 6: PARAR para o usuário commitar e dar push.** Mensagem sugerida: `feat(cli): comando spotgo auth`
