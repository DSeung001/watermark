// Package attack은 워터마크 강인성 실험에 쓰는 이미지 훼손을 구현한다.
//
// 공격은 RGB만 대상으로 한다. 입력을 먼저 불투명(alpha=255)으로 만드는데,
// JPEG에는 alpha가 없고 Go의 인코더는 premultiply를 거쳐 읽으므로 그대로 넘기면
// alpha=0 픽셀의 비트가 압축 전에 지워져 공격 효과를 분리할 수 없기 때문이다.
package attack

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"math"

	"watermark/internal/imageio"
)

// Case는 실험 조건 하나다. Level의 뜻은 Kind마다 다르다:
// jpeg=품질, blur=반경(px), resize=비율(%).
type Case struct {
	Kind  string `json:"attack"`
	Level int    `json:"level"`
	Label string `json:"label"`
}

// Cases는 실험 조건 전체다.
var Cases = []Case{
	{"jpeg", 90, "품질 90"}, {"jpeg", 70, "품질 70"}, {"jpeg", 50, "품질 50"},
	{"blur", 1, "반경 1px"}, {"blur", 2, "반경 2px"}, {"blur", 3, "반경 3px"},
	{"resize", 75, "75%"}, {"resize", 50, "50%"}, {"resize", 25, "25%"},
}

// Result는 공격 결과다. Image는 항상 원본과 같은 크기다.
type Result struct {
	Image     *image.NRGBA
	SmallW    int // resize: 축소된 크기
	SmallH    int
	JPEGBytes int // jpeg: 압축 후 파일 크기
}

// Apply는 조건 하나를 적용한다.
func Apply(img *image.NRGBA, c Case) (Result, error) {
	switch c.Kind {
	case "jpeg":
		out, n, err := JPEG(img, c.Level)
		return Result{Image: out, JPEGBytes: n}, err
	case "blur":
		return Result{Image: Blur(img, float64(c.Level))}, nil
	case "resize":
		out, small := Resize(img, float64(c.Level)/100)
		return Result{Image: out, SmallW: small.Rect.Dx(), SmallH: small.Rect.Dy()}, nil
	}
	return Result{}, fmt.Errorf("attack: unknown kind %q", c.Kind)
}

// Opaque는 RGB를 그대로 두고 alpha만 255로 채운 복사본이다.
func Opaque(img *image.NRGBA) *image.NRGBA {
	out := imageio.Clone(img)
	for i := 3; i < len(out.Pix); i += 4 {
		out.Pix[i] = 255
	}
	return out
}

// JPEG는 품질 q로 압축했다가 다시 푼 이미지와 압축 크기를 돌려준다.
func JPEG(img *image.NRGBA, q int) (*image.NRGBA, int, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, Opaque(img), &jpeg.Options{Quality: q}); err != nil {
		return nil, 0, err
	}
	size := buf.Len()
	dec, err := jpeg.Decode(&buf)
	if err != nil {
		return nil, 0, err
	}
	b := dec.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), dec, b.Min, draw.Src)
	return out, size, nil
}

// Blur는 표준편차 sigma의 분리형 Gaussian 필터다. 경계는 가장자리 값을 늘려 쓴다.
// 반경을 σ로 쓰는 것은 Pillow의 GaussianBlur(radius)와 같은 정의다.
func Blur(img *image.NRGBA, sigma float64) *image.NRGBA {
	src := Opaque(img)
	w, h := src.Rect.Dx(), src.Rect.Dy()
	r := int(math.Ceil(3 * sigma))
	kernel := make([]float64, 2*r+1)
	var sum float64
	for i := range kernel {
		d := float64(i - r)
		kernel[i] = math.Exp(-d * d / (2 * sigma * sigma))
		sum += kernel[i]
	}
	for i := range kernel {
		kernel[i] /= sum
	}

	tmp := make([]float64, w*h*3)
	for y := range h {
		for x := range w {
			for c := range 3 {
				var v float64
				for k, wt := range kernel {
					sx := clamp(x+k-r, w)
					v += wt * float64(src.Pix[(y*w+sx)*4+c])
				}
				tmp[(y*w+x)*3+c] = v
			}
		}
	}
	out := Opaque(src)
	for y := range h {
		for x := range w {
			for c := range 3 {
				var v float64
				for k, wt := range kernel {
					sy := clamp(y+k-r, h)
					v += wt * tmp[(sy*w+x)*3+c]
				}
				out.Pix[(y*w+x)*4+c] = toByte(v)
			}
		}
	}
	return out
}

// Resize는 scale배로 줄인 이미지(small)와, 그것을 원본 크기로 되돌린 이미지를 돌려준다.
// 축소는 넓이 가중 박스 평균, 복원은 바이리니어다.
func Resize(img *image.NRGBA, scale float64) (restored, small *image.NRGBA) {
	src := Opaque(img)
	w, h := src.Rect.Dx(), src.Rect.Dy()
	sw := max(int(math.Round(float64(w)*scale)), 1)
	sh := max(int(math.Round(float64(h)*scale)), 1)
	small = downscale(src, sw, sh)
	return upscale(small, w, h), small
}

func downscale(src *image.NRGBA, dw, dh int) *image.NRGBA {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	out := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	sx, sy := float64(w)/float64(dw), float64(h)/float64(dh)
	for dy := range dh {
		y0, y1 := float64(dy)*sy, float64(dy+1)*sy
		for dx := range dw {
			x0, x1 := float64(dx)*sx, float64(dx+1)*sx
			var v [3]float64
			var area float64
			for y := int(y0); y < h && float64(y) < y1; y++ {
				wy := math.Min(float64(y+1), y1) - math.Max(float64(y), y0)
				for x := int(x0); x < w && float64(x) < x1; x++ {
					a := wy * (math.Min(float64(x+1), x1) - math.Max(float64(x), x0))
					area += a
					for c := range 3 {
						v[c] += a * float64(src.Pix[(y*w+x)*4+c])
					}
				}
			}
			p := out.Pix[(dy*dw+dx)*4:]
			for c := range 3 {
				p[c] = toByte(v[c] / area)
			}
			p[3] = 255
		}
	}
	return out
}

func upscale(src *image.NRGBA, dw, dh int) *image.NRGBA {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	out := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	sx, sy := float64(w)/float64(dw), float64(h)/float64(dh)
	for dy := range dh {
		fy := (float64(dy)+0.5)*sy - 0.5
		y0 := int(math.Floor(fy))
		ty := fy - float64(y0)
		for dx := range dw {
			fx := (float64(dx)+0.5)*sx - 0.5
			x0 := int(math.Floor(fx))
			tx := fx - float64(x0)
			p := out.Pix[(dy*dw+dx)*4:]
			for c := range 3 {
				at := func(x, y int) float64 {
					return float64(src.Pix[(clamp(y, h)*w+clamp(x, w))*4+c])
				}
				top := at(x0, y0)*(1-tx) + at(x0+1, y0)*tx
				bot := at(x0, y0+1)*(1-tx) + at(x0+1, y0+1)*tx
				p[c] = toByte(top*(1-ty) + bot*ty)
			}
			p[3] = 255
		}
	}
	return out
}

func clamp(i, n int) int {
	return min(max(i, 0), n-1)
}

func toByte(v float64) uint8 {
	return uint8(math.Round(math.Min(math.Max(v, 0), 255)))
}
