package main

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"

	"github.com/mantonx/loomarr/internal/images"
	"github.com/mantonx/loomarr/internal/images/rustgen"
)

// certificationCorpusManifest describes the refusals deliberately planted in the repository
// corpus. Every other supported-looking file is expected to complete a real ladder.
type certificationCorpusManifest struct {
	ExpectedRefusals map[string]string              `json:"expectedRefusals"`
	BoundaryCases    []images.CertificationBoundary `json:"-"`
}

// writeCertificationCorpus creates deterministic, tiny inputs that exercise the V59a format and
// timeline matrix. The destination is disposable; callers should never point it at user data.
func writeCertificationCorpus(dir string) (certificationCorpusManifest, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return certificationCorpusManifest{}, err
	}
	write := func(name string, data []byte) error {
		return os.WriteFile(filepath.Join(dir, name), data, 0o600)
	}

	opaque := image.NewRGBA(image.Rect(0, 0, 640, 360))
	fillRGBA(opaque, color.RGBA{R: 24, G: 84, B: 160, A: 255})
	var jpegData bytes.Buffer
	if err := jpeg.Encode(&jpegData, opaque, &jpeg.Options{Quality: 88}); err != nil {
		return certificationCorpusManifest{}, err
	}
	if err := write("opaque-landscape.jpg", jpegData.Bytes()); err != nil {
		return certificationCorpusManifest{}, err
	}

	logo := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			logo.SetRGBA(x, y, color.RGBA{R: 220, G: 45, B: 80, A: uint8((x + y) * 2)})
		}
	}
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, logo); err != nil {
		return certificationCorpusManifest{}, err
	}
	if err := write("transparent-logo.png", pngData.Bytes()); err != nil {
		return certificationCorpusManifest{}, err
	}

	// Generated once with ffmpeg/libwebp from a 16x16 opaque green frame. Keeping the bytes fixed
	// makes the corpus independent of ffmpeg and avoids asking the worker to generate its own input.
	staticWebP, err := base64.StdEncoding.DecodeString("UklGRjgAAABXRUJQVlA4ICwAAADQAQCdASoQABAAAgA0JaACdLoB+AADsAD+8Oj3/yC5YXXI1/5sCua5+agAAA==")
	if err != nil {
		return certificationCorpusManifest{}, err
	}
	if err := write("static.webp", staticWebP); err != nil {
		return certificationCorpusManifest{}, err
	}

	if err := write("infinite-loop.gif", certificationGIF(0, 2)); err != nil {
		return certificationCorpusManifest{}, err
	}
	if err := write("finite-loop.gif", certificationGIF(2, 2)); err != nil {
		return certificationCorpusManifest{}, err
	}
	if err := write("one-frame.gif", certificationGIF(0, 1)); err != nil {
		return certificationCorpusManifest{}, err
	}
	if err := write("fractional-zero-delay.apng", certificationAPNG()); err != nil {
		return certificationCorpusManifest{}, err
	}
	if err := write("corrupt.png", append([]byte("\x89PNG\r\n\x1a\n"), []byte("truncated")...)); err != nil {
		return certificationCorpusManifest{}, err
	}
	if err := write("dimension-limit.png", dimensionOnlyPNG(20_000, 1)); err != nil {
		return certificationCorpusManifest{}, err
	}

	manifest := certificationCorpusManifest{ExpectedRefusals: map[string]string{
		"corrupt.png":         "corrupt_input",
		"dimension-limit.png": "limit_exceeded",
	}}
	manifest.BoundaryCases = certificationBoundaryCases()
	return manifest, nil
}

func certificationBoundaryCases() []images.CertificationBoundary {
	base := images.DefaultCertificationBudget()
	limit := func(name, source string, mutate func(*rustgen.Budget)) images.CertificationBoundary {
		budget := base
		mutate(&budget)
		return images.CertificationBoundary{
			Name: name, Source: source, Budget: budget,
			Targets: []rustgen.Target{}, ExpectedCode: "limit_exceeded",
		}
	}
	boundaries := []images.CertificationBoundary{
		limit("max-input-bytes", "transparent-logo.png", func(b *rustgen.Budget) { b.MaxInputBytes = 1 }),
		limit("max-width", "transparent-logo.png", func(b *rustgen.Budget) { b.MaxWidth = 1 }),
		limit("max-height", "transparent-logo.png", func(b *rustgen.Budget) { b.MaxHeight = 1 }),
		limit("max-canvas-pixels", "transparent-logo.png", func(b *rustgen.Budget) { b.MaxCanvasPixels = 1 }),
		limit("max-frames", "infinite-loop.gif", func(b *rustgen.Budget) { b.MaxFrames = 1 }),
		limit("max-total-frame-pixels", "infinite-loop.gif", func(b *rustgen.Budget) { b.MaxTotalFramePixels = 1 }),
		limit("max-duration", "infinite-loop.gif", func(b *rustgen.Budget) { b.MaxDurationMS = 1 }),
	}
	output := limit("max-output-bytes", "transparent-logo.png", func(b *rustgen.Budget) { b.MaxOutputBytes = 1 })
	output.Targets = []rustgen.Target{{ID: "webp-w32", Format: "webp", Width: 32, Motion: "first_frame"}}
	boundaries = append(boundaries, output)
	targets := make([]rustgen.Target, 17)
	for i := range targets {
		targets[i] = rustgen.Target{ID: fmt.Sprintf("webp-%02d", i), Format: "webp", Width: i + 1, Motion: "first_frame"}
	}
	boundaries = append(boundaries, images.CertificationBoundary{
		Name: "max-targets", Source: "transparent-logo.png", Budget: base,
		Targets: targets, ExpectedCode: "limit_exceeded",
	})
	return boundaries
}

func fillRGBA(img *image.RGBA, c color.RGBA) {
	for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
		for x := img.Rect.Min.X; x < img.Rect.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func certificationGIF(loopCount, frames int) []byte {
	palette := color.Palette{color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}}
	animation := &gif.GIF{LoopCount: loopCount}
	for frame := 0; frame < frames; frame++ {
		img := image.NewPaletted(image.Rect(0, 0, 16, 8), palette)
		for i := range img.Pix {
			img.Pix[i] = uint8(frame % len(palette))
		}
		animation.Image = append(animation.Image, img)
		animation.Delay = append(animation.Delay, 10)
	}
	var out bytes.Buffer
	if err := gif.EncodeAll(&out, animation); err != nil {
		panic(fmt.Sprintf("encode fixed certification GIF: %v", err))
	}
	return out.Bytes()
}

func certificationAPNG() []byte {
	const width, height = uint32(4), uint32(2)
	var out bytes.Buffer
	out.Write([]byte("\x89PNG\r\n\x1a\n"))
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], width)
	binary.BigEndian.PutUint32(ihdr[4:8], height)
	ihdr[8], ihdr[9] = 8, 6 // 8-bit RGBA
	writePNGChunk(&out, "IHDR", ihdr)
	actl := make([]byte, 8)
	binary.BigEndian.PutUint32(actl[0:4], 2)
	binary.BigEndian.PutUint32(actl[4:8], 0)
	writePNGChunk(&out, "acTL", actl)
	writePNGChunk(&out, "fcTL", frameControl(0, width, height, 1, 60))
	writePNGChunk(&out, "IDAT", compressedRGBA(width, height, color.RGBA{R: 255, A: 255}))
	writePNGChunk(&out, "fcTL", frameControl(1, width, height, 0, 100))
	fdat := make([]byte, 4)
	binary.BigEndian.PutUint32(fdat, 2)
	fdat = append(fdat, compressedRGBA(width, height, color.RGBA{B: 255, A: 255})...)
	writePNGChunk(&out, "fdAT", fdat)
	writePNGChunk(&out, "IEND", nil)
	return out.Bytes()
}

func frameControl(sequence, width, height uint32, delayNumerator, delayDenominator uint16) []byte {
	data := make([]byte, 26)
	binary.BigEndian.PutUint32(data[0:4], sequence)
	binary.BigEndian.PutUint32(data[4:8], width)
	binary.BigEndian.PutUint32(data[8:12], height)
	binary.BigEndian.PutUint16(data[20:22], delayNumerator)
	binary.BigEndian.PutUint16(data[22:24], delayDenominator)
	return data
}

func compressedRGBA(width, height uint32, c color.RGBA) []byte {
	var raw bytes.Buffer
	for range height {
		raw.WriteByte(0) // PNG filter: None
		for range width {
			raw.Write([]byte{c.R, c.G, c.B, c.A})
		}
	}
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	_, _ = zw.Write(raw.Bytes())
	_ = zw.Close()
	return compressed.Bytes()
}

func dimensionOnlyPNG(width, height uint32) []byte {
	var out bytes.Buffer
	out.Write([]byte("\x89PNG\r\n\x1a\n"))
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], width)
	binary.BigEndian.PutUint32(ihdr[4:8], height)
	ihdr[8], ihdr[9] = 8, 6
	writePNGChunk(&out, "IHDR", ihdr)
	writePNGChunk(&out, "IDAT", compressedRGBA(width, height, color.RGBA{G: 255, A: 255}))
	writePNGChunk(&out, "IEND", nil)
	return out.Bytes()
}

func writePNGChunk(out *bytes.Buffer, kind string, data []byte) {
	_ = binary.Write(out, binary.BigEndian, uint32(len(data)))
	out.WriteString(kind)
	out.Write(data)
	checksum := crc32.NewIEEE()
	_, _ = checksum.Write([]byte(kind))
	_, _ = checksum.Write(data)
	_ = binary.Write(out, binary.BigEndian, checksum.Sum32())
}
