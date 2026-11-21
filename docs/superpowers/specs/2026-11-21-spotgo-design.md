# spotGO — Design

Data: 2026-11-21
Referência: spotDL v4.6.0 (MIT), commit `58ced9af`

## Intenção

Reescrever o spotDL em Go com dois objetivos de peso igual:

1. **Aprender Go a fundo.** Cada etapa vem explicada: por que foi feita assim e qual é o jeito idiomático em Go comparado ao Python.
2. **Uso diário.** O spotDL está quebrando por limite de requisições. O spotGO precisa funcionar de forma confiável e ser mais leve e mais rápido.

## Causa do problema no spotDL

`spotdl/utils/config.py:310` tem um `client_id` fixo no código, compartilhado por todos os usuários. A cota da API estoura para todo mundo ao mesmo tempo (HTTP 429 com `Retry-After` de horas). Reescrever em Go não resolve isso. O que resolve é cada usuário usar as próprias credenciais, combinado com menos requisições (lotes + cache).

## Objetivos

- Binário único `spotgo`, instalável com `go install github.com/DeathHapyness/spotGO/cmd/spotgo@latest`.
- Credenciais do Spotify configuradas pelo próprio usuário, de forma **interativa**, no primeiro uso.
- Mesmas funções centrais do spotDL: baixar faixa, álbum, playlist, artista e busca por texto; metadados e capa embutidos; formatos mp3/m4a/opus/flac/ogg/wav; template de nome de saída; pular o que já existe; `sync` de pasta com playlist.
- Mais rápido que o spotDL (ver "Otimizações").

## Não-objetivos (por enquanto)

- Interface web (`spotdl web`).
- Login do usuário via OAuth (músicas curtidas, playlists privadas). Pode entrar depois.

## Anti-objetivos

- Embutir um `client_id` compartilhado no código. Isso repetiria a falha do spotDL.
- Baixar áudio direto do Spotify (quebrar DRM, como fazem zotify e librespot).
- Salvar o `client_secret` com permissão de leitura para outros usuários do sistema.

## Restrições

- Go 1.22 (versão instalada).
- Dependências externas em tempo de execução: `yt-dlp` (recente), `ffmpeg` e um runtime JS que o yt-dlp exige hoje para o YouTube (deno). O spotGO consegue baixá-las sozinho (ver "Dependências gerenciadas").
- Todo o código Python é **removido** do repositório. A referência continua acessível pelo histórico do git e pelo repositório upstream.
- Manter o aviso de copyright do spotDL no `LICENSE` (exigência da licença MIT, já que o projeto é derivado).
- Commits e push são feitos pelo usuário. O Claude entrega uma etapa por vez, com sugestão de mensagem de commit, e espera antes de seguir.

## Abordagens consideradas

1. **Go na raiz do repositório, com o Python removido** (escolhida). Repositório limpo, `go install` funciona direto e o histórico do fork é preservado.
2. Go numa subpasta `go/` com o Python intocado. Descartada: caminho do módulo feio e projeto com cara de Python.
3. Repositório novo. Descartada: perde o histórico do fork.

Para o download do áudio:
- **yt-dlp como processo externo atrás de uma `interface`** (escolhida). É estável porque o yt-dlp corrige mudanças do YouTube quase todo dia.
- Go puro (`kkdai/youtube`). Descartada para a v1: quebra com frequência. A `interface` permite tentar depois.

## Como funciona

### Credenciais (interativo)

Ordem de resolução:

1. Flags `--client-id` / `--client-secret`.
2. Variáveis de ambiente `SPOTGO_CLIENT_ID` / `SPOTGO_CLIENT_SECRET`.
3. Arquivo `os.UserConfigDir()/spotgo/config.json`.
4. Nada encontrado e terminal interativo: assistente de primeiro uso.
   - Explica como criar um app em developer.spotify.com/dashboard.
   - Pede o Client ID e o Client Secret. O secret é digitado sem eco (`golang.org/x/term`).
   - **Valida** pedindo um token na hora. Se falhar, mostra o erro e pede de novo.
   - Salva com permissão `0600` (pasta `0700`).
5. Nada encontrado e terminal não interativo (script, cron): erro com as instruções, sem travar esperando entrada.

Comando `spotgo auth` para refazer o assistente, mostrar o estado atual (ID mascarado) ou apagar as credenciais (`spotgo auth --clear`).

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
- **SoundCloud**: busca pela API pública v2 (o `client_id` é extraído do JS do site, como o yt-dlp faz).
- **Bandcamp**: busca via `bandcamp.com/api/bcsearch_public_api`.
- **Piped**: só se houver instância estável. A maioria das instâncias públicas caiu, então fica por último e opcional.
- Flag `--audio youtube-music,youtube,soundcloud` define a ordem de tentativa, como no spotDL.

### Pacotes

```
cmd/spotgo/        main + comandos (download, sync, auth, url, meta)
internal/config/   arquivo de config, resolução de credenciais, assistente
internal/spotify/  cliente HTTP: token, lotes, paginação, retry/429, cache
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

- **Endpoints em lote** do Spotify (`/tracks?ids=` com até 50 e `/albums?ids=` com até 20) em vez de uma requisição por faixa.
- **Cache em disco** das respostas do Spotify (`os.UserCacheDir()/spotgo`) e do token até expirar.
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
3. `internal/spotify`: token, retry/429, cache, faixa e álbum.
4. `internal/spotify`: playlist e artista (paginação), busca por texto.
5. `internal/matching` + testes.
6. `internal/provider/ytmusic`.
7. `internal/deps` + `spotgo setup` (baixar e verificar yt-dlp/ffmpeg/deno).
8. `internal/ffmpeg` + `internal/output`.
9. `internal/download`: pipeline concorrente + progresso → `spotgo download` funcionando de ponta a ponta.
10. `spotgo sync` + archive.
11. Provedores extras: YouTube, SoundCloud, Bandcamp (Piped por último).
12. Extras: letras (LRC), m3u, `meta`, `url`.

## Decisões tomadas pelo Claude (revisar)

- CLI com `spf13/cobra`: o spotDL tem ~80 flags, e escrever um parser de subcomandos na mão não é a parte que mais ensina.
- Tags via ffmpeg em vez de bibliotecas de ID3/FLAC/MP4 em Go: um caminho só para todos os formatos.
- Config em JSON (stdlib), sem YAML/TOML.
- O comando `spotgo` fica em `cmd/spotgo/`, o layout padrão para projetos Go com binário.

## Em aberto para o builder

- Pesos exatos da pontuação do matching (partir dos do spotDL).
- Formato visual da barra de progresso.
