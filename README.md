# spotGO

Baixe faixas, álbuns, playlists e artistas a partir de links do Spotify.
Reescrita em Go do [spotDL](https://github.com/spotDL/spotify-downloader):
um binário único, leve e com downloads em paralelo.

> **Status:** em desenvolvimento. Veja o [design](docs/superpowers/specs/2026-11-21-spotgo-design.md).

> spotGO **não é afiliado ao Spotify**. Os metadados vêm da API pública do
> Spotify e o áudio vem do YouTube Music. Nada é baixado do Spotify.

## Como funciona

1. Lê os metadados (título, artista, álbum, capa…) pela API do Spotify.
2. Procura a mesma faixa no YouTube Music.
3. Baixa o áudio com o `yt-dlp`, converte com o `ffmpeg` e grava as tags e a capa.

## Requisitos

- Go 1.22+ (para compilar)
- [`yt-dlp`](https://github.com/yt-dlp/yt-dlp) recente, [`ffmpeg`](https://ffmpeg.org) e [`deno`](https://deno.com)
  (o `spotgo setup` vai conseguir baixá-los para você)
- Credenciais próprias da API do Spotify (grátis, veja abaixo)

## Instalação

```bash
go install github.com/DeathHapyness/spotGO/cmd/spotgo@latest
```

## Credenciais do Spotify

O spotDL usava um Client ID compartilhado por todos os usuários. Quando o
limite da API estourava, a ferramenta quebrava para todo mundo ao mesmo tempo. O spotGO usa
**as suas** credenciais:

1. Acesse <https://developer.spotify.com/dashboard> e crie um app
   (qualquer nome; em *Redirect URI* pode usar `http://127.0.0.1:8888/callback`).
2. Copie o **Client ID** e clique em **View client secret**.
3. Rode `spotgo auth` e cole os dois. O spotgo testa as credenciais e as
   salva em `~/.config/spotgo/config.json`, com permissão `0600`.

Se ainda não houver credenciais, o spotgo pede essas informações automaticamente no primeiro uso.

Também dá para passar por flag (`--client-id`, `--client-secret`) ou por variável de
ambiente (`SPOTGO_CLIENT_ID`, `SPOTGO_CLIENT_SECRET`). A ordem de prioridade é:
flags → variáveis de ambiente → arquivo. `SPOTGO_CONFIG` troca o caminho do arquivo.

```bash
spotgo auth           # configura (ou reconfigura) as credenciais
spotgo auth --status  # mostra de onde vêm as credenciais (o secret nunca é exibido)
spotgo auth --clear   # apaga as credenciais salvas
```

## Desenvolvimento

```bash
go test -race ./...
go build -o spotgo ./cmd/spotgo
```

## Créditos e licença

MIT. Projeto derivado do [spotDL](https://github.com/spotDL/spotify-downloader)
(© 2021 spotDL Developers). Veja [LICENSE](LICENSE).
