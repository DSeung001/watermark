// Package dct는 8×8 블록 DCT 계수쌍의 대소 관계로 비트를 숨기는 워터마크다.
// (Koch–Zhao 방식)
//
// 휘도 Y의 블록마다 대칭 계수쌍 c(U1,V1), c(U2,V2)를 고른다. 비트가 1이면
// c1-c2 ≥ T, 0이면 c1-c2 ≤ -T가 되도록 두 계수를 벌린다. 블러와 리사이즈는
// 대칭인 두 계수를 거의 같은 비율로 줄이므로 차이의 부호가 유지되기 쉽다.
//
// 메시지 비트는 사용 가능한 블록 전체에 순환 반복하고 다수결로 읽는다.
// 블록 목록은 원본의 alpha에서 구하는데 공격을 거치면 alpha가 사라지므로,
// 추출하는 쪽도 원본 마스크와 메시지 길이를 알아야 한다. (semi-blind)
package dct

import (
	"errors"
	"fmt"
	"image"
	"math"

	"watermark/internal/imageio"
	"watermark/internal/vote"
)

const (
	N = 8 // 블록 한 변

	// 계수쌍. 인덱스는 V*N+U (U=가로 주파수, V=세로 주파수).
	U1, V1 = 2, 1
	U2, V2 = 1, 2

	// 평균 휘도가 이 범위를 벗어난 블록은 값이 0·255에서 잘리므로 쓰지 않는다.
	MinLuma, MaxLuma = 32, 223

	// DefaultStrength는 계수 차이 목표값 T의 기본값이다.
	DefaultStrength = 36
)

var ErrCapacity = errors.New("dct: payload exceeds block capacity")

// Block은 블록 왼쪽 위 픽셀 좌표다.
type Block struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// basis[k][n]은 k번째 1차원 DCT 기저의 n번째 값이다. (정규직교)
var basis = func() (b [N][N]float64) {
	for k := range N {
		scale := math.Sqrt(2.0 / N)
		if k == 0 {
			scale = math.Sqrt(1.0 / N)
		}
		for n := range N {
			b[k][n] = scale * math.Cos(math.Pi*(2*float64(n)+1)*float64(k)/(2*N))
		}
	}
	return b
}()

// Forward는 공간 영역 블록을 DCT 계수로 바꾼다. 인덱스는 y*N+x, 계수는 V*N+U.
func Forward(px [N * N]float64) (out [N * N]float64) {
	return transform(px, func(freq, pos int) float64 { return basis[freq][pos] })
}

// Inverse는 DCT 계수를 공간 영역으로 되돌린다.
func Inverse(co [N * N]float64) (out [N * N]float64) {
	return transform(co, func(pos, freq int) float64 { return basis[freq][pos] })
}

// 2차원 변환을 행→열 1차원 변환 두 번으로 처리한다.
func transform(in [N * N]float64, m func(a, b int) float64) (out [N * N]float64) {
	var tmp [N * N]float64
	for y := range N {
		for a := range N {
			for b := range N {
				tmp[y*N+a] += m(a, b) * in[y*N+b]
			}
		}
	}
	for x := range N {
		for a := range N {
			for b := range N {
				out[a*N+x] += m(a, b) * tmp[b*N+x]
			}
		}
	}
	return out
}

// Blocks는 삽입에 쓸 블록을 행 우선으로 돌려준다.
// 전 픽셀이 불투명하고 평균 휘도가 범위 안인 블록만 고른다.
func Blocks(orig *image.NRGBA) []Block {
	w, h := orig.Rect.Dx(), orig.Rect.Dy()
	var out []Block
	for y := 0; y+N <= h; y += N {
	next:
		for x := 0; x+N <= w; x += N {
			var sum float64
			for j := range N {
				row := orig.Pix[(y+j)*orig.Stride+x*4:]
				for i := range N {
					p := row[i*4:]
					if p[3] != 255 {
						continue next
					}
					sum += luma(p)
				}
			}
			if m := sum / (N * N); m >= MinLuma && m <= MaxLuma {
				out = append(out, Block{x, y})
			}
		}
	}
	return out
}

// Capacity는 블록당 1비트로 넣을 수 있는 최대 바이트 수다.
func Capacity(blocks []Block) int {
	return len(blocks) / 8
}

// Embed는 payload의 비트를 orig의 블록 전체에 순환 반복해 넣은 새 이미지를 돌려준다.
// 투명 픽셀과 alpha는 건드리지 않는다.
func Embed(orig *image.NRGBA, payload []byte, strength float64) (*image.NRGBA, error) {
	blocks := Blocks(orig)
	nBits := len(payload) * 8
	if nBits > len(blocks) {
		return nil, fmt.Errorf("%w: %d > %d bits", ErrCapacity, nBits, len(blocks))
	}
	out := imageio.Clone(orig)
	if nBits == 0 {
		return out, nil
	}
	for k, b := range blocks {
		co := Coefs(out, b)
		diff := co[V1*N+U1] - co[V2*N+U2]
		target := strength
		if vote.Bit(payload, k%nBits) == 0 {
			target = -strength
		}
		if (target > 0 && diff >= target) || (target < 0 && diff <= target) {
			continue // 이미 조건을 만족한다
		}
		// 두 계수를 반대로 같은 양만큼 움직여 변화량을 나눈다.
		var delta [N * N]float64
		shift := (target - diff) / 2
		delta[V1*N+U1], delta[V2*N+U2] = shift, -shift
		apply(out, b, Inverse(delta))
	}
	return out, nil
}

// Extract는 blocks에서 메시지 비트별 집계를 읽는다.
func Extract(img *image.NRGBA, blocks []Block, nBytes int) vote.Tally {
	nBits := nBytes * 8
	t := vote.New(nBits)
	if nBits == 0 {
		return t
	}
	for k, b := range blocks {
		t.Add(k%nBits, ReadBit(img, b))
	}
	return t
}

// ReadBit은 블록 하나에서 읽은 비트다.
func ReadBit(img *image.NRGBA, b Block) uint8 {
	if Diff(img, b) > 0 {
		return 1
	}
	return 0
}

// Diff는 블록의 계수 차이 c1-c2다.
func Diff(img *image.NRGBA, b Block) float64 {
	co := Coefs(img, b)
	return co[V1*N+U1] - co[V2*N+U2]
}

// Coefs는 블록 휘도의 DCT 계수다.
func Coefs(img *image.NRGBA, b Block) [N * N]float64 {
	var px [N * N]float64
	for j := range N {
		row := img.Pix[(b.Y+j)*img.Stride+b.X*4:]
		for i := range N {
			px[j*N+i] = luma(row[i*4:])
		}
	}
	return Forward(px)
}

// apply는 휘도 변화량을 R, G, B에 같게 더한다. 휘도 가중치의 합이 1이므로
// 세 채널에 같은 값을 더하면 Y도 그만큼 변한다.
func apply(img *image.NRGBA, b Block, dy [N * N]float64) {
	for j := range N {
		row := img.Pix[(b.Y+j)*img.Stride+b.X*4:]
		for i := range N {
			p := row[i*4:]
			for c := range 3 {
				v := float64(p[c]) + dy[j*N+i]
				p[c] = uint8(math.Round(math.Min(math.Max(v, 0), 255)))
			}
		}
	}
}

func luma(p []uint8) float64 {
	return 0.299*float64(p[0]) + 0.587*float64(p[1]) + 0.114*float64(p[2])
}
