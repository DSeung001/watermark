// Package metrics는 워터마크 삽입 전후 이미지 품질과 추출 정확도를 측정한다.
package metrics

import (
	"image"
	"math"
	"math/bits"
)

// MSE는 두 이미지의 RGB 채널 평균 제곱 오차다. Alpha는 제외한다.
func MSE(a, b *image.NRGBA) float64 {
	w, h := a.Rect.Dx(), a.Rect.Dy()
	var sum float64
	for y := range h {
		ra := a.Pix[y*a.Stride:]
		rb := b.Pix[y*b.Stride:]
		for x := range w {
			for c := range 3 {
				d := float64(ra[x*4+c]) - float64(rb[x*4+c])
				sum += d * d
			}
		}
	}
	return sum / float64(w*h*3)
}

// PSNR은 dB 단위 최대 신호 대 잡음비다. 두 이미지가 같으면 +Inf.
func PSNR(a, b *image.NRGBA) float64 {
	mse := MSE(a, b)
	if mse == 0 {
		return math.Inf(1)
	}
	return 10 * math.Log10(255*255/mse)
}

// SSIM은 두 이미지의 구조적 유사성(0~1, 같으면 1)이다.
// 휘도(Y = 0.299R + 0.587G + 0.114B)에 8×8 윈도우를 한 칸씩 밀며
// 계산한 SSIM의 평균을 돌려준다. 윈도우 합은 적분 영상으로 구한다.
func SSIM(a, b *image.NRGBA) float64 {
	const (
		win = 8
		c1  = (0.01 * 255) * (0.01 * 255)
		c2  = (0.03 * 255) * (0.03 * 255)
	)
	w, h := a.Rect.Dx(), a.Rect.Dy()
	if w < win || h < win {
		return math.NaN()
	}

	// 적분 영상: x, y, x², y², xy 다섯 가지 합
	iw := w + 1
	sum := make([][5]float64, iw*(h+1))
	for y := range h {
		ra := a.Pix[y*a.Stride:]
		rb := b.Pix[y*b.Stride:]
		var row [5]float64
		for x := range w {
			px := luma(ra[x*4:])
			py := luma(rb[x*4:])
			row[0] += px
			row[1] += py
			row[2] += px * px
			row[3] += py * py
			row[4] += px * py
			up := sum[y*iw+x+1]
			cur := &sum[(y+1)*iw+x+1]
			for k := range 5 {
				cur[k] = up[k] + row[k]
			}
		}
	}

	const n = win * win
	var total float64
	var count int
	for y := 0; y+win <= h; y++ {
		for x := 0; x+win <= w; x++ {
			var s [5]float64
			for k := range 5 {
				s[k] = sum[(y+win)*iw+x+win][k] - sum[y*iw+x+win][k] -
					sum[(y+win)*iw+x][k] + sum[y*iw+x][k]
			}
			mx, my := s[0]/n, s[1]/n
			vx := s[2]/n - mx*mx
			vy := s[3]/n - my*my
			cov := s[4]/n - mx*my
			total += ((2*mx*my + c1) * (2*cov + c2)) /
				((mx*mx + my*my + c1) * (vx + vy + c2))
			count++
		}
	}
	return total / float64(count)
}

func luma(p []uint8) float64 {
	return 0.299*float64(p[0]) + 0.587*float64(p[1]) + 0.114*float64(p[2])
}

// BER은 비트 오류율이다. 길이가 다르면 짧은 쪽에 없는 비트는 모두 오류로 센다.
func BER(want, got []byte) float64 {
	n := max(len(want), len(got))
	if n == 0 {
		return 0
	}
	common := min(len(want), len(got))
	errs := (n - common) * 8
	for i := range common {
		errs += bits.OnesCount8(want[i] ^ got[i])
	}
	return float64(errs) / float64(n*8)
}
