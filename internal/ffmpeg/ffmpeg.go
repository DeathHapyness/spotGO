// Package ffmpeg converte o áudio baixado e grava as tags e a capa numa
// única passada do ffmpeg.
package ffmpeg

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DeathHapyness/spotGO/internal/song"
)

type Format string

const (
	MP3  Format = "mp3"
	M4A  Format = "m4a"
	Opus Format = "opus"
	OGG  Format = "ogg"
	FLAC Format = "flac"
	WAV  Format = "wav"
)

var Formats = []Format{MP3, M4A, Opus, OGG, FLAC, WAV}

func ParseFormat(s string) (Format, error) {
	for _, f := range Formats {
		if strings.EqualFold(s, string(f)) {
			return f, nil
		}
	}
	names := make([]string, len(Formats))
	for i, f := range Formats {
		names[i] = string(f)
	}
	return "", fmt.Errorf("formato desconhecido %q (use %s)", s, strings.Join(names, ", "))
}

type spec struct {
	muxer   string
	codec   string // codec do ffmpeg para reencodar
	same    string // codec de entrada que pode ser copiado sem perda
	quality []string
}

var specs = map[Format]spec{
	MP3:  {muxer: "mp3", codec: "libmp3lame", quality: []string{"-q:a", "0"}},
	M4A:  {muxer: "ipod", codec: "aac", same: "aac", quality: []string{"-b:a", "192k"}},
	Opus: {muxer: "opus", codec: "libopus", same: "opus", quality: []string{"-b:a", "160k"}},
	OGG:  {muxer: "ogg", codec: "libvorbis", same: "vorbis", quality: []string{"-q:a", "6"}},
	FLAC: {muxer: "flac", codec: "flac"},
	WAV:  {muxer: "wav", codec: "pcm_s16le"},
}

type Job struct {
	Input string
	// InputCodec permite copiar o áudio sem reencodar (opus → .opus, aac → .m4a).
	InputCodec string
	Output     string
	Format     Format
	// Bitrate força reencodar com esse bitrate ("320k"); "" usa o padrão do formato.
	Bitrate string
	Song    song.Song
	Cover   []byte
	Lyrics  string
	// Copy copia o áudio sem reencodar, seja qual for o formato (spotgo meta).
	Copy bool
	// KeepCover mantém a capa que já está na entrada quando Cover é nil.
	KeepCover bool
	// Extra são tags da entrada que devem continuar (as que o spotgo não gerencia).
	Extra map[string]string
}

type Converter struct {
	Path string
	// Run roda o ffmpeg e devolve o stderr; nil usa os/exec.
	Run func(ctx context.Context, path string, args []string) ([]byte, error)
}

// Convert grava job.Output de forma atômica: o ffmpeg escreve num temporário
// na mesma pasta, que só é renomeado se tudo der certo.
func (c *Converter) Convert(ctx context.Context, job Job) error {
	sp, ok := specs[job.Format]
	if !ok {
		return fmt.Errorf("formato desconhecido %q", job.Format)
	}
	dir := filepath.Dir(job.Output)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	if job.KeepCover && len(job.Cover) == 0 && !attachedPic(job.Format) {
		// No ogg/opus a capa é uma tag, que o ffmpeg lê como stream de imagem:
		// extrai a imagem para gravá-la de novo na tag.
		if img, err := c.extractCover(ctx, job.Input); err == nil {
			job.Cover = img
		}
	}

	meta, err := os.CreateTemp("", "spotgo-meta-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(meta.Name())
	_, err = meta.Write(Metadata(job))
	if cerr := meta.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}

	cover := ""
	if len(job.Cover) > 0 && attachedPic(job.Format) {
		f, err := os.CreateTemp("", "spotgo-cover-*")
		if err != nil {
			return err
		}
		defer os.Remove(f.Name())
		_, err = f.Write(job.Cover)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
		cover = f.Name()
	}

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(job.Output)+".tmp-*")
	if err != nil {
		return err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	run := c.Run
	if run == nil {
		run = runFFmpeg
	}
	path := c.Path
	if path == "" {
		path = "ffmpeg"
	}
	stderr, err := run(ctx, path, args(job, sp, meta.Name(), cover, tmp.Name()))
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ffmpeg falhou em %s: %w%s", filepath.Base(job.Input), err, tail(stderr))
	}
	// O CreateTemp cria com 0600; música é para os outros programas lerem.
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), job.Output)
}

// args monta a linha de comando. cover == "" significa sem capa como stream
// (no opus e no ogg a capa vai dentro das tags).
func args(job Job, sp spec, metaPath, cover, out string) []string {
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y",
		"-i", job.Input, "-i", metaPath}
	if cover != "" {
		args = append(args, "-i", cover)
	}
	args = append(args, "-map", "0:a:0")
	switch {
	case cover != "":
		args = append(args, "-map", "2:0", "-c:v", "copy", "-disposition:v", "attached_pic")
	case job.KeepCover && attachedPic(job.Format):
		args = append(args, "-map", "0:v?", "-c:v", "copy")
	}
	args = append(args, "-map_metadata", "1", "-map_metadata:s:a", "-1", "-map_chapters", "-1")

	if job.Copy || (job.Bitrate == "" && sp.same != "" && strings.EqualFold(job.InputCodec, sp.same)) {
		args = append(args, "-c:a", "copy")
	} else {
		args = append(args, "-c:a", sp.codec)
		switch {
		case job.Bitrate != "" && job.Format != FLAC && job.Format != WAV:
			args = append(args, "-b:a", job.Bitrate)
		default:
			args = append(args, sp.quality...)
		}
	}

	switch job.Format {
	case MP3:
		args = append(args, "-id3v2_version", "3")
	case M4A:
		// Não usa +use_metadata_tags: ele gravaria o ISRC, mas faz o ffmpeg descartar a capa.
		args = append(args, "-movflags", "+faststart")
	}
	return append(args, "-f", sp.muxer, out)
}

func attachedPic(f Format) bool {
	return f == MP3 || f == M4A || f == FLAC
}

// Metadata monta o arquivo no formato FFMETADATA1. Passar as tags por arquivo
// evita o limite de tamanho de argumento (a capa em base64 passa de 128 KB).
func Metadata(job Job) []byte {
	s := job.Song
	var b bytes.Buffer
	b.WriteString(";FFMETADATA1\n")
	put := func(k, v string) {
		if v == "" {
			return
		}
		b.WriteString(escape(k))
		b.WriteByte('=')
		b.WriteString(escape(v))
		b.WriteByte('\n')
	}
	put("title", s.Title)
	put("artist", strings.Join(s.Artists, ", "))
	put("album", s.Album)
	albumArtist := s.AlbumArtist
	if albumArtist == "" {
		albumArtist = s.MainArtist()
	}
	put("album_artist", albumArtist)
	put("track", fraction(s.TrackNumber, s.TrackCount))
	put("disc", fraction(s.DiscNumber, 0))
	put("date", s.ReleaseDate)
	put("ISRC", s.ISRC)
	put("comment", s.SpotifyURL())
	put("lyrics", job.Lyrics)
	if len(job.Cover) > 0 && (job.Format == Opus || job.Format == OGG) {
		put("METADATA_BLOCK_PICTURE", pictureBlock(job.Cover))
	}
	keys := make([]string, 0, len(job.Extra))
	for k := range job.Extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if managed[strings.ToLower(k)] {
			continue
		}
		if strings.EqualFold(k, "METADATA_BLOCK_PICTURE") && len(job.Cover) > 0 {
			continue
		}
		put(k, job.Extra[k])
	}
	return b.Bytes()
}

// managed são as tags que o spotgo escreve; as da entrada com esses nomes saem.
var managed = map[string]bool{
	"title": true, "artist": true, "album": true, "album_artist": true, "track": true,
	"disc": true, "date": true, "isrc": true, "comment": true, "lyrics": true,
	"encoder": true, "duration": true,
}

func escape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '=', ';', '#', '\\', '\n':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func fraction(n, total int) string {
	switch {
	case n <= 0:
		return ""
	case total > 0:
		return strconv.Itoa(n) + "/" + strconv.Itoa(total)
	}
	return strconv.Itoa(n)
}

// pictureBlock monta o bloco PICTURE do FLAC em base64, que é como o Vorbis
// comment guarda a capa no ogg e no opus.
func pictureBlock(img []byte) string {
	mime := http.DetectContentType(img)
	var b bytes.Buffer
	w := func(v uint32) { _ = binary.Write(&b, binary.BigEndian, v) }
	w(3) // capa (frente)
	w(uint32(len(mime)))
	b.WriteString(mime)
	w(0) // descrição vazia
	w(0)
	w(0)
	w(0)
	w(0) // largura, altura, profundidade, cores: desconhecidos
	w(uint32(len(img)))
	b.Write(img)
	return base64.StdEncoding.EncodeToString(b.Bytes())
}

func runFFmpeg(ctx context.Context, path string, args []string) ([]byte, error) {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stderr = &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		err = fmt.Errorf("saiu com código %d", exitErr.ExitCode())
	}
	return stderr.Bytes(), err
}

func tail(stderr []byte) string {
	lines := strings.Split(strings.TrimSpace(string(stderr)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return ""
	}
	if len(lines) > 3 {
		lines = lines[len(lines)-3:]
	}
	return ":\n  " + strings.Join(lines, "\n  ")
}

type Info struct {
	// Tags com as chaves em minúsculas.
	Tags     map[string]string
	Duration time.Duration
	Codec    string
	HasCover bool
}

var (
	durationRe = regexp.MustCompile(`Duration: (\d+):(\d+):(\d+(?:\.\d+)?)`)
	audioRe    = regexp.MustCompile(`Stream #\S+: Audio: (\w+)`)
)

// Probe lê as tags, a duração e o codec só com o ffmpeg (o ffprobe nem
// sempre está instalado). No ogg e no opus as tags ficam no stream de áudio.
func (c *Converter) Probe(ctx context.Context, path string, f Format) (Info, error) {
	args := []string{"-hide_banner", "-nostdin", "-i", path}
	if f == Opus || f == OGG {
		args = append(args, "-map_metadata", "0:s:a:0")
	}
	args = append(args, "-f", "ffmetadata", "-")
	bin := c.Path
	if bin == "" {
		bin = "ffmpeg"
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return Info{}, ctx.Err()
		}
		return Info{}, fmt.Errorf("ffmpeg não conseguiu ler %s%s", filepath.Base(path), tail(stderr.Bytes()))
	}
	return parseProbe(stdout.String(), stderr.String()), nil
}

func parseProbe(meta, info string) Info {
	out := Info{Tags: ParseMetadata(meta), HasCover: strings.Contains(info, "(attached pic)")}
	if m := durationRe.FindStringSubmatch(info); m != nil {
		h, _ := strconv.Atoi(m[1])
		mi, _ := strconv.Atoi(m[2])
		sec, _ := strconv.ParseFloat(m[3], 64)
		out.Duration = time.Duration((float64(h*3600+mi*60) + sec) * float64(time.Second))
	}
	if m := audioRe.FindStringSubmatch(info); m != nil {
		out.Codec = m[1]
	}
	return out
}

// ParseMetadata lê o formato FFMETADATA1 (só a seção global).
func ParseMetadata(s string) map[string]string {
	tags := map[string]string{}
	var line strings.Builder
	flush := func() {
		l := line.String()
		line.Reset()
		if l == "" || strings.HasPrefix(l, ";") || strings.HasPrefix(l, "#") {
			return
		}
		k, v, ok := cutUnescaped(l)
		if ok {
			tags[strings.ToLower(k)] = v
		}
	}
	for i := 0; i < len(s); i++ {
		switch ch := s[i]; {
		case ch == '\\' && i+1 < len(s):
			line.WriteByte('\\')
			line.WriteByte(s[i+1])
			i++
		case ch == '\n':
			flush()
			if strings.HasPrefix(s[i+1:], "[") {
				return tags
			}
		default:
			line.WriteByte(ch)
		}
	}
	flush()
	return tags
}

// cutUnescaped separa "chave=valor" no primeiro = sem barra e tira os escapes.
func cutUnescaped(l string) (string, string, bool) {
	var k, v strings.Builder
	cur, found := &k, false
	for i := 0; i < len(l); i++ {
		switch {
		case l[i] == '\\' && i+1 < len(l):
			i++
			cur.WriteByte(l[i])
		case l[i] == '=' && !found:
			found, cur = true, &v
		default:
			cur.WriteByte(l[i])
		}
	}
	return k.String(), v.String(), found
}

func (c *Converter) extractCover(ctx context.Context, path string) ([]byte, error) {
	bin := c.Path
	if bin == "" {
		bin = "ffmpeg"
	}
	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, "-hide_banner", "-nostdin", "-loglevel", "error",
		"-i", path, "-map", "0:v:0", "-c:v", "copy", "-f", "image2pipe", "-")
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	if stdout.Len() == 0 {
		return nil, errors.New("sem capa")
	}
	return stdout.Bytes(), nil
}
