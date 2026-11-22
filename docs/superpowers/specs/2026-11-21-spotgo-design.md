# spotGO — Design

Data: 2026-11-21 (revisado em 2026-11-22: regras da Web API de fev/2026; modo padrão sem Premium)
Referência: spotDL v4.6.0 (MIT), commit `58ced9af`

## Intenção

Reescrever o spotDL em Go com dois objetivos de peso igual:

1. **Aprender Go a fundo.** Cada etapa vem explicada: por que foi feita assim e qual é o jeito idiomático em Go comparado ao Python.
2. **Uso diário.** O spotDL está quebrando por limite de requisições. O spotGO precisa funcionar de forma confiável e ser mais leve e mais rápido.

## Causa do problema no spotDL

`spotdl/utils/config.py:310` tem um `client_id` fixo no código, compartilhado por todos os usuários. A cota da API estoura para todo mundo ao mesmo tempo (HTTP 429 com `Retry-After` de horas). Reescrever em Go não resolve isso. O que resolve é cada usuário usar as próprias credenciais, combinado com menos requisições (cache + limitador).

## Objetivos

- Binário único `spotgo`, instalável com `go install github.com/DeathHapyness/spotGO/cmd/spotgo@latest`.
- **Funciona sem conta, sem Premium e sem credenciais** (modo padrão: embed do Spotify + Deezer).
- **Modo avançado opcional** para quem tem Premium: credenciais próprias (`spotgo auth`) + login (`spotgo login`). Dá playlists completas e metadados exatos.
- Mesmas funções centrais do spotDL: baixar faixa, álbum, playlist, artista e busca por texto; metadados e capa embutidos; formatos mp3/m4a/opus/flac/ogg/wav; template de nome de saída; pular o que já existe; `sync` de pasta com playlist.
- Mais rápido que o spotDL (ver "Otimizações").

## Não-objetivos (por enquanto)

- Interface web (`spotdl web`).
- Músicas curtidas (`/me/tracks`). O login existe, mas este recurso fica para depois.

## Anti-objetivos

- Embutir um `client_id` compartilhado no código. Isso repetiria a falha do spotDL.
- Baixar áudio direto do Spotify (quebrar DRM, como fazem zotify e librespot).
- Salvar o `client_secret` ou o refresh token com permissão de leitura para outros usuários do sistema.
- Usar a API privada do web player (`api-partner.spotify.com/pathfinder`, token anônimo via TOTP, como o spotipyFree/SpotAPI do spotDL 4.6). Avaliada e **descartada** em 2026-11-22: é acesso não autorizado, viola os termos e depende de segredos extraídos do JS do player.
- Extrair chaves ou segredos do JavaScript de sites (Spotify, SoundCloud etc.) no nosso próprio código.

## Restrições

- Go 1.22 (versão instalada).
- Dependências externas em tempo de execução: `yt-dlp` (recente), `ffmpeg` e um runtime JS que o yt-dlp exige hoje para o YouTube (deno). O spotGO consegue baixá-las sozinho (ver "Dependências gerenciadas").
- Todo o código Python é **removido** do repositório. A referência continua acessível pelo histórico do git e pelo repositório upstream.
- Manter o aviso de copyright do spotDL no `LICENSE` (exigência da licença MIT, já que o projeto é derivado).
- **Regras da Web API para apps em Development Mode (fev/2026):**
  - O dono do app precisa ter **Spotify Premium** ativo (o usuário tem).
  - **Endpoints em lote removidos** (`GET /tracks`, `/albums`, `/artists` com `?ids=`). É uma requisição por item.
  - `GET /artists/{id}/top-tracks` foi removido. O download de artista usa `GET /artists/{id}/albums`.
  - `GET /playlists/{id}/tracks` virou `GET /playlists/{id}/items` e **só devolve itens de playlists do usuário logado ou em que ele colabora**. Para as outras playlists vêm só os metadados, sem `items`.
  - `GET /search`: `limit` máximo 10 (padrão 5).
  - Campos removidos: `popularity`, `available_markets`, `linked_from` (faixa); `label`, `popularity`, `album_group`, `available_markets` (álbum); `followers`, `popularity` (artista). `external_ids` (ISRC) **voltou** em mar/2026.
  - A cota é contada **por conta de desenvolvedor**, não por Client ID (jul/2026). O 429 vem com `reason: QUOTA_EXCEEDED`.
- Commits e push são feitos pelo usuário. O Claude entrega uma etapa por vez, com sugestão de mensagem de commit, e espera antes de seguir.

## Abordagens consideradas

1. **Go na raiz do repositório, com o Python removido** (escolhida). Repositório limpo, `go install` funciona direto e o histórico do fork é preservado.
2. Go numa subpasta `go/` com o Python intocado. Descartada: caminho do módulo feio e projeto com cara de Python.
3. Repositório novo. Descartada: perde o histórico do fork.

Fonte dos metadados:
- **D como padrão + C opcional** (escolhida em 2026-11-22). O padrão usa o embed do Spotify para a lista de faixas e a API pública do Deezer para os metadados. Não precisa de Premium nem de configuração. Com credenciais + login, usa a API oficial (C).
- Só C. Descartada como padrão: exige Premium do usuário e configuração antes do primeiro uso.
- Só D. Descartada: perderia playlists completas (> 100) para quem tem Premium, e o trabalho de credenciais já feito.

Detalhe de C, para ler playlists que não são do usuário (a API oficial não devolve os itens delas):
- **Híbrido**: API oficial com credenciais próprias + login OAuth para as playlists do usuário + **página pública de embed** (`open.spotify.com/embed/playlist/{id}`) para as playlists de outras pessoas. O embed é uma página pública feita para incorporação, que traz `trackList` no `__NEXT_DATA__` sem token. Verificado em 2026-11-22: funciona, mas **limita a 100 faixas**.
- Só a API oficial. Descartada: não lê playlists de outras pessoas, que o usuário precisa.
- API privada do web player. Descartada (ver Anti-objetivos).

Para o download do áudio:
- **yt-dlp como processo externo atrás de uma `interface`** (escolhida). É estável porque o yt-dlp corrige mudanças do YouTube quase todo dia.
- Go puro (`kkdai/youtube`). Descartada para a v1: quebra com frequência. A `interface` permite tentar depois.

## Como funciona

### Modos de fonte (`--source auto|public|api`)

- `public`: embed do Spotify + Deezer. Nenhuma credencial.
- `api`: API oficial. Exige credenciais. Se não houver, roda o assistente (com TTY) ou mostra `ErrNoCredentials`.
- `auto` (padrão): `api` se houver credenciais configuradas (flags/env/arquivo); senão, `public`. **Nunca abre o assistente sozinho.** Quem não configurou nada nem fica sabendo que credenciais existem, a não ser por uma dica quando uma playlist bate 100 faixas.

### Modo público: embed + Deezer

```
URL do Spotify
  → embed (open.spotify.com/embed/{track|album|playlist|artist}/{id}) → __NEXT_DATA__
      faixa:    título, artistas, duração, data
      álbum:    nome, artista, trackList (título, artistas, duração, ordem)
      playlist: nome, trackList (≤ 100)
      artista:  nome (o trackList é só o top 10 → discografia vem do Deezer)
  → Deezer (api.deezer.com, sem chave)
      busca `artist:"…" track:"…"` (+ `album:"…"` quando conhecido) → candidatos
      escolhe por: título normalizado, artista, |duração| ≤ 3 s, preferindo um álbum de mesmo nome e evitando coletâneas
      GET /track/{id} → ISRC, track_position, disk_number, release_date, álbum, cover_xl (1000×1000)
      álbum: GET /album/{id} uma vez → todas as faixas (sem uma chamada por faixa)
  → []Song
```

- **Faixa sem correspondência no Deezer** → segue com os dados do embed (título, artistas, duração), sem capa/álbum, e com aviso. Nunca descarta a faixa.
- **Cota do Deezer**: ~50 requisições a cada 5 s por IP → limitador no cliente (`golang.org/x/time/rate` ou token bucket próprio). Um erro `{"error":{"code":4}}` ("Quota limit exceeded") → espera e tenta de novo.
- **Playlist com exatamente 100 faixas** no modo público → aviso: o Spotify só expõe 100 faixas pelo embed. Com Premium: `spotgo auth` + `spotgo login` e copiar a playlist para uma sua. Sem Premium: dividir em playlists de até 100.
- Embed ou Deezer com formato inesperado → erro claro, sem panic. Os testes usam fixtures salvas.

### Credenciais (modo api)

Ordem de resolução:

1. Flags `--client-id` / `--client-secret`.
2. Variáveis de ambiente `SPOTGO_CLIENT_ID` / `SPOTGO_CLIENT_SECRET`.
3. Arquivo `os.UserConfigDir()/spotgo/config.json`.
4. Nada encontrado, `--source api` e terminal interativo: assistente.
   - Explica como criar um app em developer.spotify.com/dashboard.
   - Pede o Client ID e o Client Secret. O secret é digitado sem eco (`golang.org/x/term`).
   - **Valida** pedindo um token na hora. Se falhar, mostra o erro e pede de novo.
   - Salva com permissão `0600` (pasta `0700`).
5. Nada encontrado, `--source api` e terminal não interativo (script, cron): erro com as instruções, sem travar esperando entrada.
6. Nada encontrado e `--source auto`: modo público, sem perguntar nada.

Comando `spotgo auth` para refazer o assistente, mostrar o estado atual (ID mascarado) ou apagar as credenciais (`spotgo auth --clear`). **Já implementado.**

### Login do usuário (`spotgo login`)

- OAuth Authorization Code **com PKCE**, usando o Client ID do usuário. O servidor local de callback escuta em `127.0.0.1:8888/callback`, a mesma Redirect URI que o README manda cadastrar.
- `state` aleatório conferido no callback. O servidor fecha depois do primeiro callback ou após um timeout de 5 min.
- Escopos: `playlist-read-private playlist-read-collaborative`.
- Abre o navegador (`xdg-open`/`open`/`start`). Se não conseguir, imprime a URL para o usuário abrir.
- O refresh token é salvo no mesmo `config.json` (`0600`). O access token é renovado sozinho.
- `spotgo logout` apaga o refresh token.

### Resolução de playlist

```
playlist URL
  ├─ logado? → GET /playlists/{id}/items (paginado; funciona se é dono/colaborador)
  │            se a API devolver sem items (não é do usuário) ↓
  └─ embed   → open.spotify.com/embed/playlist/{id} → __NEXT_DATA__.trackList (≤ 100)
               → IDs das faixas → GET /tracks/{id} (um por faixa, com cache) → []Song
```

- O embed dá `uri`, `title`, `subtitle` (artistas) e `duration`. Os metadados completos (álbum, capa, número da faixa, ISRC) vêm de `GET /tracks/{id}`.
- Embed com **exatamente 100 faixas** → aviso: "esta playlist pode ter mais de 100 faixas; o Spotify só expõe 100 para playlists de outras pessoas. Para baixar inteira, copie-a para uma playlist sua no app (selecionar tudo → Adicionar à playlist) e rode `spotgo login`."
- Se o formato do `__NEXT_DATA__` mudar → erro claro dizendo que a leitura pelo embed quebrou (sem panic). O teste usa uma página salva como fixture.

### Fluxo do `download`

```
entrada (URL/busca)
  → resolver (Spotify API: faixa/álbum/playlist/artista → []Song)
  → match (busca no YouTube Music + pontuação → URL do vídeo)
  → download (yt-dlp → arquivo de áudio temporário)
  → convert+tag (UMA passada do ffmpeg: formato + metadados + capa)
  → arquivo final no template de saída
```

Cada estágio é um pool de goroutines ligado por channels, com `context.Context` para cancelar tudo no Ctrl+C e limites de concorrência por estágio (rede ≠ CPU).

### Dependências gerenciadas (yt-dlp, ffmpeg, deno)

- `spotgo setup` baixa os binários oficiais para `os.UserCacheDir()/spotgo/bin`:
  - yt-dlp: releases do GitHub, conferido com o `SHA2-256SUMS` publicado.
  - deno: releases do GitHub, conferido com o `.sha256sum`.
  - ffmpeg: build estático (BtbN no GitHub), conferido com o checksum publicado.
- **Ordem de busca de cada binário:** flag (`--ffmpeg`, `--yt-dlp`) → pasta gerenciada → `PATH`.
- Se faltar algum binário no `download`, o spotgo **pergunta** antes de baixar (`--yes` pula a pergunta). Num terminal não interativo, mostra o erro com a dica `spotgo setup`.
- Checksum que não bate = o arquivo é apagado e nada é executado.
- **yt-dlp desatualizado**: avisa quando a versão tem mais de ~60 dias. `spotgo setup --update` atualiza. O yt-dlp do apt (2024.04.09 nesta máquina) é exatamente o caso que quebra.
- Binários são baixados para `arquivo.tmp` e renomeados só depois de verificados (escrita atômica).

### Provedores de áudio

Interface `AudioProvider` com `Search(ctx, Song) ([]Result, error)`. O download continua sempre pelo yt-dlp, que já suporta YouTube, SoundCloud e Bandcamp. Cada provedor novo só precisa implementar a **busca**.

- **YouTube Music** (padrão, innertube).
- **YouTube** comum (innertube), como fallback quando o YT Music não acha.
- **SoundCloud**: busca delegada ao yt-dlp (`scsearch5:<consulta>`). Não extraímos o `client_id` do JS do site no nosso código (ver Anti-objetivos).
- **Bandcamp**: busca via `bandcamp.com/api/bcsearch_public_api`.
- **Piped**: só se houver instância estável. A maioria das instâncias públicas caiu, então fica por último e opcional.
- Flag `--audio youtube-music,youtube,soundcloud` define a ordem de tentativa, como no spotDL.

### Pacotes

```
cmd/spotgo/        main + comandos (download, sync, auth, url, meta)
internal/config/   arquivo de config, resolução de credenciais, assistente
internal/spotify/  cliente HTTP: token, OAuth PKCE, paginação, retry/429, limitador, cache
internal/fetch/    cliente HTTP compartilhado: retry com Retry-After, limitador por host, cache em disco
internal/embed/    leitura da página pública de embed (faixa, álbum, playlist ≤ 100, artista)
internal/deezer/   busca e metadados no Deezer + escolha do candidato
internal/source/   escolhe o modo (auto/public/api) e resolve uma URL em []Song
internal/song/     tipo Song e afins (o que era spotdl/types)
internal/provider/ interface AudioProvider + ytmusic, youtube, soundcloud, bandcamp, piped
internal/deps/     localizar, baixar, verificar e atualizar yt-dlp/ffmpeg/deno
internal/matching/ pontuação de resultados (porte de spotdl/utils/matching.py)
internal/download/ pipeline concorrente, progresso, pular existentes
internal/ffmpeg/   conversão + metadados + capa numa passada
internal/output/   template de nome de arquivo, sanitização
internal/sync/     estado de sync, diff playlist × pasta
```

## Otimizações em relação ao spotDL

- ~~Endpoints em lote~~: removidos pelo Spotify em fev/2026. O ganho agora vem de:
- **Cache em disco** das respostas do Spotify (`os.UserCacheDir()/spotgo`), principalmente de `GET /tracks/{id}` e `GET /albums/{id}`, que quase nunca mudam. Um `sync` repetido não gasta cota. O token fica em cache até expirar.
- **Reaproveitar a resposta do álbum**: `GET /albums/{id}` já traz as faixas (`tracks.items`). Num download de álbum, não há uma chamada por faixa.
- **Retry que respeita o `Retry-After`** com backoff e jitter, mais um limitador de taxa no cliente para evitar o 429 antes que ele aconteça.
- **Pipeline por estágios** com concorrência separada: busca e download limitados pela rede, conversão limitada por `runtime.NumCPU()`.
- **ffmpeg uma vez só** por faixa: conversão, tags e capa juntas.
- **Pular cedo**: se o arquivo final já existe (ou consta no arquivo de archive), a faixa nem entra na busca.
- **Busca no YouTube Music via HTTP direto** (API innertube) em vez de abrir um processo do yt-dlp para cada busca.
- Binário único, sem tempo de inicialização de interpretador.

## Testes

- Cliente Spotify e ytmusic com `net/http/httptest` (sem rede real).
- Matching em testes de tabela, portando casos de `tests/` do spotDL.
- Template de saída e sanitização com testes de tabela.
- Um teste de integração opcional (build tag `integration`) que usa a rede de verdade.

## Etapas (cada uma = um commit do usuário)

1. Remover o Python, criar `go.mod`, esqueleto do `cmd/spotgo`, `.gitignore` e README novos.
2. `internal/config` + assistente interativo + `spotgo auth`.
3. `internal/song` (tipo `Song`, parse de URL do Spotify) + `internal/fetch` (retry/429 com `Retry-After`, limitador por host, cache em disco).
4. `internal/embed`: faixa, álbum, playlist (≤ 100) e artista.
5. `internal/deezer`: busca, escolha do candidato, faixa e álbum, discografia do artista.
6. `internal/source` modo `public` + `spotgo url`/`meta` mínimos para ver o resultado. README: credenciais viram opcionais.
7. `internal/spotify` (modo api): token em cache, faixa, álbum, artista (álbuns) e busca (limite 10).
8. `spotgo login`/`logout` (OAuth PKCE) + playlists do usuário + o modo `auto`.
9. `internal/matching` + testes.
10. `internal/provider/ytmusic`.
11. `internal/deps` + `spotgo setup` (baixar e verificar yt-dlp/ffmpeg/deno).
12. `internal/ffmpeg` + `internal/output`.
13. `internal/download`: pipeline concorrente + progresso → `spotgo download` funcionando de ponta a ponta.
14. `spotgo sync` + archive.
15. Provedores extras: YouTube, SoundCloud, Bandcamp (Piped por último).
16. Extras: letras (LRC), m3u.

## Decisões tomadas pelo Claude (revisar)

- CLI com `spf13/cobra`: o spotDL tem ~80 flags, e escrever um parser de subcomandos na mão não é a parte que mais ensina.
- Tags via ffmpeg em vez de bibliotecas de ID3/FLAC/MP4 em Go: um caminho só para todos os formatos.
- Config em JSON (stdlib), sem YAML/TOML.
- O comando `spotgo` fica em `cmd/spotgo/`, o layout padrão para projetos Go com binário.

## Em aberto para o builder

- Pesos exatos da pontuação do matching (partir dos do spotDL).
- Formato visual da barra de progresso.
