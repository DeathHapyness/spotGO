<div align="center">

# spotGO

**spotGO** encontra no YouTube Music as músicas das suas playlists do Spotify e as baixa com capa, letra e metadados.

[![MIT License](https://img.shields.io/github/license/DeathHapyness/spotGO?color=44CC11&style=flat-square)](LICENSE)
[![Go version](https://img.shields.io/github/go-mod/go-version/DeathHapyness/spotGO?color=44CC11&style=flat-square)](go.mod)
[![CI](https://img.shields.io/github/actions/workflow/status/DeathHapyness/spotGO/go.yml?branch=go-rewrite&label=testes&style=flat-square)](https://github.com/DeathHapyness/spotGO/actions/workflows/go.yml)
![GitHub Repo stars](https://img.shields.io/github/stars/DeathHapyness/spotGO?style=flat-square)

> spotGO: o spotDL reescrito em Go. Um binário só, sem Python, sem conta e sem credenciais.

</div>

______________________________________________________________________

> [!WARNING]
> **Em desenvolvimento.** O `spotgo download` e o `spotgo sync` já baixam faixas, álbuns, playlists e
> artistas com capa e metadados, e o `spotgo meta` arruma as tags de músicas que você já tem; o `login` ainda não existe. Acompanhe o [design](docs/superpowers/specs/2026-11-21-spotgo-design.md)
> e a lista de [operações](#operações) abaixo.

______________________________________________________________________

## Por que spotGO?

O spotDL tem um `client_id` da API do Spotify embutido no código e **compartilhado por todos os
usuários**. Quando a cota estoura, a ferramenta quebra para todo mundo ao mesmo tempo
(HTTP 429 com espera de horas). O spotGO resolve isso assim:

- **Não precisa de conta nem de credenciais.** A lista de faixas vem da página pública de embed do
  Spotify e os metadados (álbum, número da faixa, ISRC, capa 1000×1000) vêm da API pública do Deezer.
- **Faz menos requisições:** cache em disco, limite de taxa por site e retry que respeita o `Retry-After`.
- **É leve:** binário único em Go, sem Python. O download é em paralelo, com uma única passada do ffmpeg por faixa
  (e sem reencodar quando o formato do YouTube já serve, como no `--format opus`).
- **Credenciais próprias são opcionais**, para quem tem Spotify Premium e quer playlists com mais de 100 faixas
  (veja [Modo avançado](#modo-avançado-opcional)).

## Instalação

### Go (método recomendado)

- Instale com `go install github.com/DeathHapyness/spotGO/cmd/spotgo@latest`.
- Para atualizar, rode o mesmo comando de novo.

  > O binário vai para `$(go env GOPATH)/bin`. Confira se essa pasta está no seu `PATH`.

<details>
    <summary style="font-size:1.25em"><strong>Outras opções</strong></summary>

- Compilar a partir do código-fonte

  ```bash
  git clone https://github.com/DeathHapyness/spotGO && cd spotGO
  go build -o spotgo ./cmd/spotgo
  ```

  O executável `spotgo` é criado na pasta do projeto.

- Executáveis prontos
  - Em breve, na aba [Releases](https://github.com/DeathHapyness/spotGO/releases).

</details>

### yt-dlp, FFmpeg e Deno

O spotGO usa o [`yt-dlp`](https://github.com/yt-dlp/yt-dlp) para baixar o áudio do YouTube, o
[`ffmpeg`](https://ffmpeg.org) para converter e gravar as tags, e o [`deno`](https://deno.com), que o
yt-dlp exige hoje para alguns vídeos do YouTube.

O jeito mais fácil é rodar:

```sh
spotgo setup            # baixa o que faltar e confere o SHA-256 de cada um
spotgo setup --update   # baixa de novo as versões mais recentes
```

Os binários oficiais vão para uma pasta só do spotgo (`~/.cache/spotgo/bin` no Linux) e têm prioridade
sobre os do sistema. O que já está instalado é aproveitado, exceto um yt-dlp com mais de 60 dias, que
costuma quebrar com o YouTube (é o caso do yt-dlp do apt). Se preferir instalar no sistema:

- Linux: `sudo apt install ffmpeg` (ou o gerenciador de pacotes da sua distro). Para o yt-dlp, prefira
  o [release oficial](https://github.com/yt-dlp/yt-dlp/releases) ou o `spotgo setup`.
- macOS: `brew install yt-dlp ffmpeg deno`. Não há build oficial do ffmpeg para Mac com checksum
  publicado, então o `spotgo setup` pede o `brew install ffmpeg`.
- Windows: `winget install yt-dlp.yt-dlp Gyan.FFmpeg DenoLand.Deno`

## Uso

Uso geral:

```sh
spotgo [operação] [opções] LINKS...
```

Os **links** podem ser de faixas, álbuns, playlists ou artistas do Spotify
(`https://open.spotify.com/...` ou `spotify:album:...`). Qualquer outro texto vira **busca**: o spotgo
usa a faixa mais relevante do Deezer, evitando coletâneas (`spotgo download "queen bohemian rhapsody"`). Para ver todas as **opções**, use `spotgo -h`
ou `spotgo [operação] -h`.

Exemplos:

```sh
# baixa uma faixa como "Artista - Título.mp3" na pasta atual
spotgo download https://open.spotify.com/track/2EqlS6tkEnglzr7tkKAAYD

# por busca, sem link
spotgo download "rick astley never gonna give you up"

# um álbum em opus, separado em pastas
spotgo download --format opus -o "{album-artist}/{album}/{track-number} {title}" \
  https://open.spotify.com/album/0ETFjACtuP2ADo6LFhL6HN

# uma playlist numerada, numa pasta com o nome dela
spotgo download -o "{list-name}/{list-position} - {artists} - {title}" \
  https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M

# mantém uma pasta igual a uma playlist (na 2ª vez, basta "spotgo sync" na pasta)
spotgo sync --dir ~/Música/Mix -o "{list-position} - {artists} - {title}" \
  https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M

# letra sincronizada no arquivo e num .lrc ao lado, e uma playlist .m3u8 do álbum
spotgo download --lyrics tag,lrc --m3u https://open.spotify.com/album/0ETFjACtuP2ADo6LFhL6HN

# se o YouTube Music e o YouTube não acharem, tenta o SoundCloud e o Bandcamp
spotgo download --audio youtube-music,youtube,soundcloud,bandcamp <link>

# arruma as tags de uma pasta de músicas (veja antes com --dry-run)
spotgo meta --dry-run ~/Música
spotgo meta ~/Música

# metadados de um álbum, em texto
spotgo save https://open.spotify.com/album/0ETFjACtuP2ADo6LFhL6HN

# metadados de uma playlist, em JSON
spotgo save --json https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M | jq '.[0].songs[].isrc'

# link do YouTube Music de cada faixa de um álbum
spotgo url https://open.spotify.com/album/0ETFjACtuP2ADo6LFhL6HN
```

### Operações

<details open>
<summary style="font-size:1em"><strong>Operações suportadas</strong></summary>

| Operação | O que faz | Estado |
|---|---|---|
| `save` | Mostra os metadados (título, artistas, álbum, duração, ISRC) sem baixar nada. `--json` para usar com `jq`. | ✅ pronto |
| `auth` | Configura as credenciais próprias da API do Spotify (modo avançado). | ✅ pronto |
| `download` | Baixa as faixas do YouTube Music com capa, letra ([LRCLIB](https://lrclib.net), sincronizada quando existe) e metadados (mp3, m4a, opus, ogg, flac, wav). `--m3u` grava a playlist. Pula as que já existem; `--archive` guarda as já baixadas. | ✅ pronto |
| `url` | Mostra o link do áudio encontrado para cada faixa (YouTube Music, YouTube, SoundCloud ou Bandcamp, pelo `--audio`). `--json` para usar com `jq`. | ✅ pronto |
| `sync` | Sincroniza uma pasta com uma playlist: baixa as faixas novas, renomeia as que mudaram de posição e apaga as removidas (só arquivos que ele mesmo baixou). | ✅ pronto |
| `meta` | Atualiza as tags, a capa e a letra de músicas que você já tem, sem reencodar o áudio. Acha a faixa pelo link do Spotify no comentário, pelas tags ou pelo nome do arquivo. `--dry-run` só mostra. | ✅ pronto |
| `login` | Login na sua conta (Premium) para ler playlists suas com mais de 100 faixas. | 🚧 em breve |
| `setup` | Baixa e confere o yt-dlp, o ffmpeg e o deno. `--update` atualiza. | ✅ pronto |

</details>

<details>
<summary style="font-size:1em"><strong>Limites do modo sem credenciais</strong></summary>

- **Playlists de outras pessoas mostram no máximo 100 faixas.** É um limite da página de embed do Spotify.
  O spotgo avisa quando uma playlist bate esse limite.
- **Faixas que não estão no Deezer** saem só com título, artistas e duração, sem álbum, número da faixa e
  capa em alta. Elas nunca são descartadas, e o spotgo avisa quantas foram.
- **Artistas:** a discografia vem do Deezer, que pode ter um catálogo um pouco diferente do Spotify. Entram
  só os álbuns; singles e EPs entram apenas quando o artista não tem nenhum álbum.

</details>

## Modo avançado (opcional)

> [!NOTE]
> **Ainda não é usado.** Por enquanto, `spotgo auth` só guarda e testa as credenciais; todas as operações
> usam o modo sem credenciais. O uso delas (com `spotgo login`) chega numa próxima etapa.

Exige **Spotify Premium**: desde fevereiro de 2026, a Web API do Spotify só funciona para apps cujo dono
tenha Premium. Ninguém precisa disso para usar o spotgo. Vai ser útil para quem quer playlists próprias com
mais de 100 faixas e metadados exatos do Spotify.

1. Acesse <https://developer.spotify.com/dashboard> e crie um app
   (qualquer nome; em *Redirect URI* use `http://127.0.0.1:8888/callback`).
2. Copie o **Client ID** e clique em **View client secret**.
3. Rode `spotgo auth` e cole os dois. O spotgo testa as credenciais e as
   salva em `~/.config/spotgo/config.json`, com permissão `0600`.

Também dá para passar por flag (`--client-id`, `--client-secret`) ou por variável de
ambiente (`SPOTGO_CLIENT_ID`, `SPOTGO_CLIENT_SECRET`). A ordem de prioridade é:
flags → variáveis de ambiente → arquivo. `SPOTGO_CONFIG` troca o caminho do arquivo.

```bash
spotgo auth           # configura (ou reconfigura) as credenciais
spotgo auth --status  # mostra de onde vêm as credenciais (o secret nunca é exibido)
spotgo auth --clear   # apaga as credenciais salvas
```

## Origem da música e qualidade do áudio

O spotGO usa o YouTube Music como fonte do áudio. **Nada é baixado do Spotify**, e o spotGO não quebra DRM
nem usa a API privada do player do Spotify. Os metadados vêm da página pública de embed do Spotify e da API
pública do Deezer.

> [!NOTE]
> Os usuários são responsáveis pelo que fazem e pelas consequências legais. Não apoiamos o download não
> autorizado de material protegido por direitos autorais e não nos responsabilizamos pelo uso que cada um fizer.

### Qualidade do áudio

O spotGO baixa sempre o melhor áudio disponível no YouTube: 128 kbps para usuários comuns e 256 kbps para
assinantes do YouTube Music Premium. Converter para um formato "maior" (por exemplo, flac) não melhora a
qualidade.

> spotGO **não é afiliado ao Spotify, ao Deezer nem ao YouTube**.

## Contribuindo

Quer ajudar? O projeto é escrito em Go 1.22, sem dependências de runtime além das ferramentas acima.

```bash
go test -race ./...                       # testes (sem rede)
go test -tags integration ./internal/...  # testes que usam a rede de verdade
go build -o spotgo ./cmd/spotgo
```

O design e os planos de cada etapa ficam em [`docs/superpowers/`](docs/superpowers/).

## Licença

Este projeto usa a licença [MIT](LICENSE). Ele é derivado do
[spotDL](https://github.com/spotDL/spotify-downloader) (© 2021 spotDL Developers), que também é MIT.
