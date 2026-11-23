// Package deps localiza, baixa e confere os binários externos do spotgo
// (yt-dlp, ffmpeg e deno).
package deps

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Tool string

const (
	YTDLP  Tool = "yt-dlp"
	FFmpeg Tool = "ffmpeg"
	Deno   Tool = "deno"
)

var Tools = []Tool{YTDLP, FFmpeg, Deno}

var ErrUnsupported = errors.New("não há build oficial para esta plataforma")

type Archive int

const (
	Raw Archive = iota
	Zip
	TarXZ
)

type Asset struct {
	Name    string
	URL     string
	Archive Archive
	// Member é o sufixo do caminho do binário dentro do arquivo ("" em Raw).
	Member string
}

// Source diz de onde baixar o checksum e como escolher o asset a partir dele.
type Source struct {
	SumsURL string
	Pick    func(sums map[string]string) (Asset, error)
}

var (
	ytdlpBase  = "https://github.com/yt-dlp/yt-dlp/releases/latest/download"
	denoBase   = "https://github.com/denoland/deno/releases/latest/download"
	ffmpegBase = "https://github.com/BtbN/FFmpeg-Builds/releases/download/latest"
)

type platform struct{ goos, goarch string }

var ytdlpAssets = map[platform]string{
	{"linux", "amd64"}:   "yt-dlp_linux",
	{"linux", "arm64"}:   "yt-dlp_linux_aarch64",
	{"darwin", "amd64"}:  "yt-dlp_macos",
	{"darwin", "arm64"}:  "yt-dlp_macos",
	{"windows", "amd64"}: "yt-dlp.exe",
	{"windows", "arm64"}: "yt-dlp_arm64.exe",
}

var denoTriples = map[platform]string{
	{"linux", "amd64"}:   "x86_64-unknown-linux-gnu",
	{"linux", "arm64"}:   "aarch64-unknown-linux-gnu",
	{"darwin", "amd64"}:  "x86_64-apple-darwin",
	{"darwin", "arm64"}:  "aarch64-apple-darwin",
	{"windows", "amd64"}: "x86_64-pc-windows-msvc",
	{"windows", "arm64"}: "aarch64-pc-windows-msvc",
}

// Plataforma do BtbN e extensão do arquivo.
var ffmpegTargets = map[platform][2]string{
	{"linux", "amd64"}:   {"linux64", ".tar.xz"},
	{"linux", "arm64"}:   {"linuxarm64", ".tar.xz"},
	{"windows", "amd64"}: {"win64", ".zip"},
	{"windows", "arm64"}: {"winarm64", ".zip"},
}

func BinaryName(t Tool, goos string) string {
	if goos == "windows" {
		return string(t) + ".exe"
	}
	return string(t)
}

func unsupported(t Tool, goos, goarch, hint string) error {
	return fmt.Errorf("%s em %s/%s: %w%s", t, goos, goarch, ErrUnsupported, hint)
}

func SourceFor(t Tool, goos, goarch string) (Source, error) {
	p := platform{goos, goarch}
	switch t {
	case YTDLP:
		name, ok := ytdlpAssets[p]
		if !ok {
			return Source{}, unsupported(t, goos, goarch, "")
		}
		return Source{
			SumsURL: ytdlpBase + "/SHA2-256SUMS",
			Pick: func(sums map[string]string) (Asset, error) {
				if _, ok := sums[name]; !ok {
					return Asset{}, fmt.Errorf("yt-dlp: %s não está na lista de checksums", name)
				}
				return Asset{Name: name, URL: ytdlpBase + "/" + name, Archive: Raw}, nil
			},
		}, nil

	case Deno:
		triple, ok := denoTriples[p]
		if !ok {
			return Source{}, unsupported(t, goos, goarch, "")
		}
		name := "deno-" + triple + ".zip"
		return Source{
			SumsURL: denoBase + "/" + name + ".sha256sum",
			// O .sha256sum tem um hash só; o nome dentro dele não importa.
			Pick: func(sums map[string]string) (Asset, error) {
				if len(sums) != 1 {
					return Asset{}, fmt.Errorf("deno: esperava 1 checksum em %s.sha256sum, achei %d", name, len(sums))
				}
				return Asset{Name: name, URL: denoBase + "/" + name, Archive: Zip, Member: BinaryName(Deno, goos)}, nil
			},
		}, nil

	case FFmpeg:
		target, ok := ffmpegTargets[p]
		if !ok {
			hint := ""
			if goos == "darwin" {
				hint = "; instale com brew install ffmpeg"
			}
			return Source{}, unsupported(t, goos, goarch, hint)
		}
		return Source{
			SumsURL: ffmpegBase + "/checksums.sha256",
			Pick: func(sums map[string]string) (Asset, error) {
				name, ok := pickFFmpeg(sums, target[0], target[1])
				if !ok {
					return Asset{}, fmt.Errorf("ffmpeg: nenhum build estável para %s na lista de checksums", target[0])
				}
				archive := TarXZ
				if target[1] == ".zip" {
					archive = Zip
				}
				return Asset{Name: name, URL: ffmpegBase + "/" + name, Archive: archive, Member: "bin/" + BinaryName(FFmpeg, goos)}, nil
			},
		}, nil
	}
	return Source{}, fmt.Errorf("ferramenta desconhecida: %s", t)
}

var ffmpegStable = regexp.MustCompile(`^ffmpeg-n(\d+)\.(\d+)-latest-([a-z0-9]+)-gpl-\d+\.\d+(\.tar\.xz|\.zip)$`)

// pickFFmpeg escolhe a maior versão estável (comparação numérica: n10.0 > n9.0).
func pickFFmpeg(sums map[string]string, target, ext string) (string, bool) {
	best, bestMajor, bestMinor := "", -1, -1
	for name := range sums {
		m := ffmpegStable.FindStringSubmatch(name)
		if m == nil || m[3] != target || m[4] != ext {
			continue
		}
		major, _ := strconv.Atoi(m[1])
		minor, _ := strconv.Atoi(m[2])
		if major > bestMajor || (major == bestMajor && minor > bestMinor) {
			best, bestMajor, bestMinor = name, major, minor
		}
	}
	return best, best != ""
}

// ParseSums lê o formato do sha256sum ("<hash>  <nome>" ou "<hash> *<nome>")
// e a saída do Get-FileHash do PowerShell, que o deno usa no Windows.
func ParseSums(data []byte) (map[string]string, error) {
	if bytes.Contains(data, []byte("Algorithm :")) || bytes.Contains(data, []byte("Algorithm:")) {
		return parsePowerShell(data)
	}
	sums := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		hash, name, ok := strings.Cut(line, " ")
		name = strings.TrimPrefix(strings.TrimSpace(name), "*")
		if !ok || name == "" {
			return nil, fmt.Errorf("linha de checksum inválida: %q", line)
		}
		hash, err := checkHash(name, hash)
		if err != nil {
			return nil, err
		}
		sums[name] = hash
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(sums) == 0 {
		return nil, errors.New("lista de checksums vazia")
	}
	return sums, nil
}

func checkHash(name, hash string) (string, error) {
	hash = strings.ToLower(hash)
	if b, err := hex.DecodeString(hash); err != nil || len(b) != 32 {
		return "", fmt.Errorf("checksum inválido para %s: %q", name, hash)
	}
	return hash, nil
}

func parsePowerShell(data []byte) (map[string]string, error) {
	var hash, path string
	for _, line := range strings.Split(string(data), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "Hash":
			hash = strings.TrimSpace(v)
		case "Path":
			path = strings.TrimSpace(v)
		}
	}
	name := path[strings.LastIndexAny(path, `\/`)+1:]
	hash, err := checkHash(name, hash)
	if err != nil {
		return nil, err
	}
	return map[string]string{name: hash}, nil
}
