// Package media 负责识别上传文件的真实类型，并把浏览器无法直接显示的
// 图片格式（HEIC/HEIF、AVIF、BMP、TIFF）转成 JPEG。
package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"math"

	"github.com/gen2brain/avif"
	"github.com/gen2brain/heic"
	"golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	"golang.org/x/image/tiff"

	_ "golang.org/x/image/webp"
	_ "image/gif"
	_ "image/png"
)

type Kind string

const (
	KindImage Kind = "image"
	KindVideo Kind = "video"
)

// Format 是服务端识别出的真实文件类型。
type Format struct {
	Kind      Kind
	Ext       string
	Transcode bool // 需要转码成 JPEG 才能在所有浏览器中显示
}

var (
	ErrTooLarge    = errors.New("media: 图片尺寸过大")
	ErrUnsupported = errors.New("media: 不支持的图片格式")
)

const (
	maxPixels     = 60_000_000
	maxDimension  = 12_000
	outputMaxEdge = 2560
	jpegQuality   = 88
)

var (
	jpegMagic  = []byte{0xFF, 0xD8, 0xFF}
	pngMagic   = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	gifMagic   = []byte("GIF8")
	bmpMagic   = []byte("BM")
	tiffLittle = []byte{0x49, 0x49, 0x2A, 0x00}
	tiffBig    = []byte{0x4D, 0x4D, 0x00, 0x2A}
	ebmlMagic  = []byte{0x1A, 0x45, 0xDF, 0xA3}
)

// Detect 依据文件头魔数判断类型，不信任客户端给的 Content-Type 和文件名。
func Detect(prefix []byte) (Format, bool) {
	switch {
	case bytes.HasPrefix(prefix, jpegMagic):
		return Format{Kind: KindImage, Ext: ".jpg"}, true
	case bytes.HasPrefix(prefix, pngMagic):
		return Format{Kind: KindImage, Ext: ".png"}, true
	case bytes.HasPrefix(prefix, gifMagic):
		return Format{Kind: KindImage, Ext: ".gif"}, true
	case len(prefix) >= 12 && string(prefix[0:4]) == "RIFF" && string(prefix[8:12]) == "WEBP":
		return Format{Kind: KindImage, Ext: ".webp"}, true
	case bytes.HasPrefix(prefix, bmpMagic):
		return Format{Kind: KindImage, Ext: ".bmp", Transcode: true}, true
	case bytes.HasPrefix(prefix, tiffLittle), bytes.HasPrefix(prefix, tiffBig):
		return Format{Kind: KindImage, Ext: ".tif", Transcode: true}, true
	case bytes.HasPrefix(prefix, ebmlMagic):
		return Format{Kind: KindVideo, Ext: ".webm"}, true
	}

	if brands, ok := ftypBrands(prefix); ok {
		return formatFromBrands(brands)
	}

	// 少数 .mov 文件没有 ftyp box，直接以 QuickTime 顶层 atom 开头。
	if len(prefix) >= 8 {
		switch string(prefix[4:8]) {
		case "moov", "mdat", "wide", "free":
			return Format{Kind: KindVideo, Ext: ".mov"}, true
		}
	}
	return Format{}, false
}

// ftypBrands 解析 ISO BMFF 的 ftyp box，返回 major brand 与 compatible brands。
func ftypBrands(prefix []byte) ([]string, bool) {
	if len(prefix) < 12 || string(prefix[4:8]) != "ftyp" {
		return nil, false
	}
	size := int(binary.BigEndian.Uint32(prefix[:4]))
	if size <= 0 || size > len(prefix) {
		size = len(prefix)
	}
	brands := []string{string(prefix[8:12])}
	for offset := 16; offset+4 <= size; offset += 4 {
		brand := string(prefix[offset : offset+4])
		if brand == "" {
			continue
		}
		brands = append(brands, brand)
	}
	return brands, true
}

func formatFromBrands(brands []string) (Format, bool) {
	has := func(values ...string) bool {
		for _, brand := range brands {
			for _, value := range values {
				if brand == value {
					return true
				}
			}
		}
		return false
	}

	switch {
	case has("avif", "avis"):
		return Format{Kind: KindImage, Ext: ".avif", Transcode: true}, true
	case has("heic", "heix", "hevc", "hevx", "heim", "heis", "hevm", "hevs", "mif1", "msf1"):
		return Format{Kind: KindImage, Ext: ".heic", Transcode: true}, true
	case has("qt  "):
		return Format{Kind: KindVideo, Ext: ".mov"}, true
	case has("isom", "iso2", "iso3", "iso4", "iso5", "iso6", "mp41", "mp42",
		"avc1", "M4V ", "M4VH", "M4VP", "dash", "mmp4", "MSNV", "f4v ",
		"3gp4", "3gp5", "3g2a"):
		return Format{Kind: KindVideo, Ext: ".mp4"}, true
	}
	return Format{}, false
}

// TranscodeToJPEG 把非浏览器通用格式解码并转成 JPEG，超长边会等比缩小。
func TranscodeToJPEG(data []byte, ext string) ([]byte, error) {
	var width, height int
	var err error

	switch ext {
	case ".heic":
		var cfg image.Config
		cfg, err = heic.DecodeConfig(bytes.NewReader(data))
		width, height = cfg.Width, cfg.Height
	case ".avif":
		var cfg image.Config
		cfg, err = avif.DecodeConfig(bytes.NewReader(data))
		width, height = cfg.Width, cfg.Height
	case ".bmp":
		var cfg image.Config
		cfg, err = bmp.DecodeConfig(bytes.NewReader(data))
		width, height = cfg.Width, cfg.Height
	case ".tif":
		var cfg image.Config
		cfg, err = tiff.DecodeConfig(bytes.NewReader(data))
		width, height = cfg.Width, cfg.Height
	default:
		return nil, ErrUnsupported
	}
	if err != nil {
		return nil, fmt.Errorf("解析图片尺寸失败: %w", err)
	}
	if width <= 0 || height <= 0 || width > maxDimension || height > maxDimension ||
		int64(width)*int64(height) > maxPixels {
		return nil, ErrTooLarge
	}

	var img image.Image
	switch ext {
	case ".heic":
		img, err = heic.Decode(bytes.NewReader(data))
	case ".avif":
		img, err = avif.Decode(bytes.NewReader(data), avif.Options{AutoRotate: true})
	case ".bmp":
		img, err = bmp.Decode(bytes.NewReader(data))
	case ".tif":
		img, err = tiff.Decode(bytes.NewReader(data))
	}
	if err != nil {
		return nil, fmt.Errorf("解码图片失败: %w", err)
	}

	img = scaleDown(img, outputMaxEdge)

	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, fmt.Errorf("编码图片失败: %w", err)
	}
	return out.Bytes(), nil
}

func scaleDown(src image.Image, maxEdge int) image.Image {
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= maxEdge && height <= maxEdge {
		return src
	}

	ratio := float64(maxEdge) / float64(max(width, height))
	dstWidth := max(1, int(math.Round(float64(width)*ratio)))
	dstHeight := max(1, int(math.Round(float64(height)*ratio)))

	dst := image.NewRGBA(image.Rect(0, 0, dstWidth, dstHeight))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Src, nil)
	return dst
}
