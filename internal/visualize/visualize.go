// Package visualize는 블로그용 시각화 이미지를 만든다.
package visualize

import (
	"image"
	"image/color"
)

// BitPlane은 RGB 각 채널의 bit번째 비트를 0 또는 255로 펼친 이미지다.
// bit=0이면 LSB 평면이 되어 숨긴 데이터가 놓인 영역이 드러난다.
func BitPlane(img *image.NRGBA, bit uint) *image.NRGBA {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			s := img.Pix[y*img.Stride+x*4:]
			var px [3]uint8
			for c := range 3 {
				if (s[c]>>bit)&1 == 1 {
					px[c] = 255
				}
			}
			out.SetNRGBA(x, y, color.NRGBA{px[0], px[1], px[2], 255})
		}
	}
	return out
}

// AmplifiedDiff는 RGB 채널별 |a-b|에 gain을 곱해(255에서 포화) 보여준다.
func AmplifiedDiff(a, b *image.NRGBA, gain int) *image.NRGBA {
	w, h := a.Rect.Dx(), a.Rect.Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			pa := a.Pix[y*a.Stride+x*4:]
			pb := b.Pix[y*b.Stride+x*4:]
			var px [3]uint8
			for c := range 3 {
				d := int(pa[c]) - int(pb[c])
				if d < 0 {
					d = -d
				}
				px[c] = uint8(min(d*gain, 255))
			}
			out.SetNRGBA(x, y, color.NRGBA{px[0], px[1], px[2], 255})
		}
	}
	return out
}
