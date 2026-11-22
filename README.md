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
> **Em desenvolvimento.** Por enquanto o spotGO já lê os metadados (`spotgo save`), mas ainda
> não baixa o áudio. Acompanhe o [design](docs/superpowers/specs/2026-11-21-spotgo-design.md)
> e a lista de [operações](#operações) abaixo.

______________________________________________________________________

## Por que spotGO?

O spotDL tem um `client_id` da API do Spotify embutido no código e **compartilhado por todos os
usuários**. Quando a cota estoura, a ferramenta quebra para todo mundo ao mesmo tempo
(HTTP 429 com espera de horas). O spotGO resolve isso assim:

- **Não precisa de conta nem de credenciais.** A lista de faixas vem da página pública de embed do
  Spotify e os metadados (álbum, número da faixa, ISRC, capa 1000×1000) vêm da API pública do Deezer.
- **Faz menos requisições:** cache em disco, limite de taxa por site e retry que respeita o `Retry-After`.
- **É mais rápido e mais leve:** binário único em Go, downloads em paralelo e uma única passada do ffmpeg por faixa.
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

Em breve, `spotgo setup` vai baixá-los e conferir o checksum de cada um, guardando tudo numa pasta só do
spotgo. Se preferir instalar no sistema:

- Linux: `sudo apt install ffmpeg` (ou o gerenciador de pacotes da sua distro). O yt-dlp do apt costuma
  estar **desatualizado** e quebra com o YouTube; prefira o [release oficial](https://github.com/yt-dlp/yt-dlp/releases).
- macOS: `brew install yt-dlp ffmpeg deno`
- Windows: `winget install yt-dlp.yt-dlp Gyan.FFmpeg DenoLand.Deno`

## Uso

Uso geral:

```sh
spotgo [operação] [opções] LINKS...
```

Os **links** podem ser de faixas, álbuns, playlists ou artistas do Spotify
(`https://open.spotify.com/...` ou `spotify:album:...`). Para ver todas as **opções**, use `spotgo -h`
ou `spotgo [operação] -h`.

Exemplos:

```sh
# metadados de um álbum, em texto
spotgo save https://open.spotify.com/album/0ETFjACtuP2ADo6LFhL6HN

# metadados de uma playlist, em JSON
spotgo save --json https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M | jq '.[0].songs[].isrc'
```

### Operações

<details open>
<summary style="font-size:1em"><strong>Operações suportadas</strong></summary>

| Operação | O que faz | Estado |
|---|---|---|
| `save` | Mostra os metadados (título, artistas, álbum, duração, ISRC) sem baixar nada. `--json` para usar com `jq`. | ✅ pronto |
| `auth` | Configura as credenciais próprias da API do Spotify (modo avançado). | ✅ pronto |
| `download` | Baixa as faixas do YouTube Music com capa e metadados. | 🚧 em breve |
| `url` | Mostra a URL do áudio encontrado no YouTube Music para cada faixa. | 🚧 em breve |
| `sync` | Sincroniza uma pasta com uma playlist: baixa as faixas novas e apaga as removidas. | 🚧 em breve |
| `meta` | Atualiza os metadados de arquivos de música que você já tem. | 🚧 em breve |
| `login` | Login na sua conta (Premium) para ler playlists suas com mais de 100 faixas. | 🚧 em breve |
| `setup` | Baixa e confere o yt-dlp, o ffmpeg e o deno. | 🚧 em breve |

</details>

<details>
<summary style="font-size:1em"><strong>Limites do modo sem credenciais</strong></summary>

- **Playlists de outras pessoas mostram no máximo 100 faixas.** É um limite da página de embed do Spotify.
  O spotgo avisa quando uma playlist bate esse limite.
- **Faixas que não estão no Deezer** saem só com título, artistas e duração, sem álbum, número da faixa e
  capa em alta. Elas nunca são descartadas, e o spotgo avisa quantas foram.
- **Artistas:** a discografia vem do Deezer, que pode ter um catálogo um pouco diferente do Spotify.

</details>

## Modo avançado (opcional)

Exige **Spotify Premium**: desde fevereiro de 2026, a Web API do Spotify só funciona para apps cujo dono
tenha Premium. Ninguém precisa disso para usar o spotgo. É útil para quem quer playlists próprias com mais
de 100 faixas e metadados exatos do Spotify.

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
