// Package lsb는 LSB(Least Significant Bit) 치환 방식의 워터마크를 구현한다.
//
// 페이로드 포맷: [4바이트 big-endian 길이][데이터]
// 삽입 순서: 픽셀 행 우선, 각 픽셀의 R→G→B 채널 LSB (Alpha는 건드리지 않음),
// 각 바이트는 MSB부터 기록한다.
package lsb

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"

	"watermark/internal/imageio"
	"watermark/internal/vote"
)

const headerBytes = 4

var ErrCapacity = errors.New("lsb: payload exceeds image capacity")

// Capacity는 이미지에 숨길 수 있는 최대 페이로드 바이트 수(헤더 제외)다.
func Capacity(img *image.NRGBA) int {
	b := img.Bounds()
	c := b.Dx()*b.Dy()*3/8 - headerBytes
	return max(c, 0)
}

// Embed는 img를 복제한 뒤 payload를 LSB에 기록한 새 이미지를 반환한다.
func Embed(img *image.NRGBA, payload []byte) (*image.NRGBA, error) {
	if len(payload) > Capacity(img) {
		return nil, fmt.Errorf("%w: %d > %d bytes", ErrCapacity, len(payload), Capacity(img))
	}
	out := imageio.Clone(img)

	data := make([]byte, headerBytes+len(payload))
	binary.BigEndian.PutUint32(data, uint32(len(payload)))
	copy(data[headerBytes:], payload)

	ch := newChannels(out)
	for _, by := range data {
		for i := 7; i >= 0; i-- {
			bit := (by >> i) & 1
			idx := ch.next()
			out.Pix[idx] = out.Pix[idx]&^1 | bit
		}
	}
	return out, nil
}

// Extract는 Embed로 숨긴 페이로드를 꺼낸다.
func Extract(img *image.NRGBA) ([]byte, error) {
	ch := newChannels(img)
	header := readBytes(img, ch, headerBytes)
	n := int(binary.BigEndian.Uint32(header))
	if n > Capacity(img) {
		return nil, fmt.Errorf("%w: header says %d bytes, capacity %d", ErrCapacity, n, Capacity(img))
	}
	return readBytes(img, ch, n), nil
}

// Positions는 분산 삽입에 쓰는 자리를 순서대로 돌려준다. alpha=255인 픽셀의
// R, G, B 채널이며, 값은 촘촘한 Stride(폭*4) 기준 Pix 인덱스다.
// 공격을 거치면 alpha가 사라지므로 추출할 때도 원본에서 구한 목록을 쓴다.
func Positions(orig *image.NRGBA) []int {
	w, h := orig.Rect.Dx(), orig.Rect.Dy()
	pos := make([]int, 0, w*h*3)
	for y := range h {
		row := orig.Pix[y*orig.Stride:]
		for x := range w {
			if row[x*4+3] != 255 {
				continue
			}
			base := (y*w + x) * 4
			pos = append(pos, base, base+1, base+2)
		}
	}
	return pos
}

// FrameBits는 [길이 헤더][데이터] 프레임 하나의 비트 수다.
func FrameBits(payloadLen int) int {
	return (headerBytes + payloadLen) * 8
}

// Frame은 [4바이트 길이][데이터] 프레임을 만든다.
func Frame(payload []byte) []byte {
	data := make([]byte, headerBytes+len(payload))
	binary.BigEndian.PutUint32(data, uint32(len(payload)))
	copy(data[headerBytes:], payload)
	return data
}

// EmbedSpread는 프레임을 불투명 픽셀 전체의 LSB에 처음부터 끝까지 순환 반복한다.
// 메시지 비트 하나가 이미지 곳곳의 여러 자리에 나뉘어 들어간다.
func EmbedSpread(img *image.NRGBA, payload []byte) (*image.NRGBA, error) {
	pos := Positions(img)
	frameBits := FrameBits(len(payload))
	if frameBits > len(pos) {
		return nil, fmt.Errorf("%w: %d > %d bits", ErrCapacity, frameBits, len(pos))
	}
	out := imageio.Clone(img)
	frame := Frame(payload)
	for i, idx := range pos {
		out.Pix[idx] = out.Pix[idx]&^1 | vote.Bit(frame, i%frameBits)
	}
	return out, nil
}

// ExtractSpread는 positions의 LSB를 읽어 메시지 비트별로 집계한다.
// 공격 후에는 헤더를 믿을 수 없으므로 메시지 길이 n을 인자로 받는다.
func ExtractSpread(img *image.NRGBA, positions []int, n int) vote.Tally {
	t := vote.New(n * 8)
	frameBits := FrameBits(n)
	for i, idx := range positions {
		if k := i%frameBits - headerBytes*8; k >= 0 {
			t.Add(k, img.Pix[idx]&1)
		}
	}
	return t
}

// SpreadHeader는 첫 프레임의 길이 헤더를 읽는다.
func SpreadHeader(img *image.NRGBA, positions []int) uint32 {
	var n uint32
	for _, idx := range positions[:min(headerBytes*8, len(positions))] {
		n = n<<1 | uint32(img.Pix[idx]&1)
	}
	return n
}

// BitStep은 비트 하나가 어느 픽셀·채널에 어떻게 기록됐는지를 나타낸다. (시각화용)
type BitStep struct {
	X       int    `json:"x"`
	Y       int    `json:"y"`
	Channel string `json:"ch"`
	Alpha   uint8  `json:"a"`
	Before  uint8  `json:"before"`
	After   uint8  `json:"after"`
	Bit     uint8  `json:"bit"`
	Section string `json:"section"` // "header" | "payload"
	ByteIdx int    `json:"byte"`    // 헤더/페이로드 각 구역 내 바이트 인덱스
}

// Trace는 orig와 Embed 결과 wm을 같은 순서로 순회하며 payloadLen 바이트
// (헤더 포함)가 기록된 과정을 비트 단위로 돌려준다.
func Trace(orig, wm *image.NRGBA, payloadLen int) []BitStep {
	total := (headerBytes + payloadLen) * 8
	steps := make([]BitStep, 0, total)
	ch := newChannels(orig)
	for i := range total {
		x, y, c := ch.x, ch.y, ch.c
		idx := ch.next()
		s := BitStep{
			X: x, Y: y,
			Channel: string("RGB"[c]),
			Alpha:   orig.Pix[idx-c+3],
			Before:  orig.Pix[idx],
			After:   wm.Pix[idx],
			Bit:     wm.Pix[idx] & 1,
			Section: "payload",
			ByteIdx: i/8 - headerBytes,
		}
		if i < headerBytes*8 {
			s.Section, s.ByteIdx = "header", i/8
		}
		steps = append(steps, s)
	}
	return steps
}

func readBytes(img *image.NRGBA, ch *channels, n int) []byte {
	buf := make([]byte, n)
	for k := range buf {
		var by byte
		for range 8 {
			by = by<<1 | img.Pix[ch.next()]&1
		}
		buf[k] = by
	}
	return buf
}

// channels는 Pix 슬라이스에서 R, G, B 채널 인덱스를 순서대로 돌려준다.
// Stride가 폭*4보다 클 수 있는 서브이미지도 올바르게 순회한다.
type channels struct {
	img  *image.NRGBA
	x, y int // 현재 픽셀 (Rect 기준 오프셋)
	c    int // 0=R, 1=G, 2=B
}

func newChannels(img *image.NRGBA) *channels {
	return &channels{img: img}
}

func (ch *channels) next() int {
	idx := ch.y*ch.img.Stride + ch.x*4 + ch.c
	ch.c++
	if ch.c == 3 {
		ch.c = 0
		ch.x++
		if ch.x == ch.img.Rect.Dx() {
			ch.x = 0
			ch.y++
		}
	}
	return idx
}
