// Package imageio는 PNG 이미지를 non-premultiplied NRGBA로 읽고 쓴다.
//
// 워터마크 비트를 픽셀에 직접 쓰기 때문에 premultiplied RGBA(image.RGBA)로
// 변환하면 안 된다. alpha=0인 픽셀은 premultiply 과정에서 RGB가 0으로
// 바뀌어 숨겨둔 비트가 사라진다.
package imageio

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"os"
	"path/filepath"
)

// Load는 PNG 파일을 읽어 *image.NRGBA로 반환한다.
func Load(path string) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, err := Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return img, nil
}

// Decode는 PNG 스트림을 읽어 *image.NRGBA로 반환한다.
func Decode(r io.Reader) (*image.NRGBA, error) {
	src, err := png.Decode(r)
	if err != nil {
		return nil, err
	}
	if n, ok := src.(*image.NRGBA); ok {
		return n, nil
	}
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst, nil
}

// Save는 이미지를 PNG로 저장한다. 상위 디렉터리가 없으면 만든다.
func Save(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return fmt.Errorf("encode %s: %w", path, err)
	}
	return f.Close()
}

// Clone은 NRGBA 이미지를 (0,0) 기준의 촘촘한 Stride로 깊은 복사한다.
func Clone(img *image.NRGBA) *image.NRGBA {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		copy(out.Pix[y*out.Stride:(y+1)*out.Stride], img.Pix[y*img.Stride:])
	}
	return out
}
