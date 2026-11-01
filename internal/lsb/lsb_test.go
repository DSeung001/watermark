package lsb

import (
	"bytes"
	"errors"
	"image"
	"math/rand/v2"
	"path/filepath"
	"testing"

	"watermark/internal/imageio"
)

func randomImage(w, h int, seed uint64) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	r := rand.New(rand.NewPCG(seed, seed))
	for i := range img.Pix {
		img.Pix[i] = uint8(r.UintN(256))
	}
	return img
}

func TestRoundTrip(t *testing.T) {
	img := randomImage(64, 48, 1)
	full := bytes.Repeat([]byte{0xA5}, Capacity(img))
	cases := map[string][]byte{
		"empty": {},
		"text":  []byte("hello watermark"),
		"full":  full,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			wm, err := Embed(img, payload)
			if err != nil {
				t.Fatal(err)
			}
			got, err := Extract(wm)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, payload) {
				t.Fatalf("payload mismatch: got %d bytes, want %d", len(got), len(payload))
			}
		})
	}
}

func TestCapacityExceeded(t *testing.T) {
	img := randomImage(16, 16, 2)
	_, err := Embed(img, make([]byte, Capacity(img)+1))
	if !errors.Is(err, ErrCapacity) {
		t.Fatalf("want ErrCapacity, got %v", err)
	}
}

func TestPixelChangeBounded(t *testing.T) {
	img := randomImage(32, 32, 3)
	wm, err := Embed(img, bytes.Repeat([]byte{0xFF, 0x00}, Capacity(img)/2))
	if err != nil {
		t.Fatal(err)
	}
	for i := range img.Pix {
		d := int(img.Pix[i]) - int(wm.Pix[i])
		if i%4 == 3 && d != 0 {
			t.Fatalf("alpha changed at %d", i)
		}
		if d < -1 || d > 1 {
			t.Fatalf("pixel %d changed by %d", i, d)
		}
	}
}

// 실제 투명 배경 PNG로 저장→재로딩 후에도 추출되는지 확인한다.
func TestGithubPNGRoundTrip(t *testing.T) {
	img, err := imageio.Load(filepath.Join("..", "..", "assets", "github_01.png"))
	if err != nil {
		t.Skip("asset not found:", err)
	}
	payload := []byte("LSB survives PNG save/load, even in transparent pixels")
	wm, err := Embed(img, payload)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "wm.png")
	if err := imageio.Save(path, wm); err != nil {
		t.Fatal(err)
	}
	reloaded, err := imageio.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Extract(reloaded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("got %q, want %q", got, payload)
	}
}

// 왼쪽 절반만 불투명하고 오른쪽은 투명 검정인 이미지
func halfOpaque(w, h int, seed uint64) *image.NRGBA {
	img := randomImage(w, h, seed)
	for y := range h {
		for x := range w {
			p := img.Pix[img.PixOffset(x, y):]
			p[3] = 255
			if x >= w/2 {
				p[0], p[1], p[2], p[3] = 0, 0, 0, 0
			}
		}
	}
	return img
}

func TestSpreadRoundTrip(t *testing.T) {
	img := halfOpaque(64, 48, 5)
	payload := []byte("spread watermark")
	wm, err := EmbedSpread(img, payload)
	if err != nil {
		t.Fatal(err)
	}
	pos := Positions(img)
	tally := ExtractSpread(wm, pos, len(payload))
	if got := tally.Bytes(); !bytes.Equal(got, payload) {
		t.Fatalf("got %q", got)
	}
	if errs, total := tally.Errors(payload); errs != 0 || total == 0 {
		t.Fatalf("errs=%d total=%d", errs, total)
	}
	if n := SpreadHeader(wm, pos); n != uint32(len(payload)) {
		t.Fatalf("header = %d", n)
	}
	copies := len(pos) / FrameBits(len(payload))
	if tally.Copies[0] < copies {
		t.Fatalf("bit 0 has %d copies, want at least %d", tally.Copies[0], copies)
	}
}

func TestSpreadOnlyTouchesOpaqueLSB(t *testing.T) {
	img := halfOpaque(32, 32, 6)
	wm, err := EmbedSpread(img, []byte{0xFF, 0x00, 0xA5})
	if err != nil {
		t.Fatal(err)
	}
	for y := range 32 {
		for x := range 32 {
			for c := range 4 {
				o := img.PixOffset(x, y) + c
				d := int(img.Pix[o]) - int(wm.Pix[o])
				switch {
				case (c == 3 || x >= 16) && d != 0:
					t.Fatalf("(%d,%d) ch %d changed by %d", x, y, c, d)
				case d < -1 || d > 1:
					t.Fatalf("(%d,%d) ch %d changed by %d", x, y, c, d)
				}
			}
		}
	}
}

func TestSpreadCapacityExceeded(t *testing.T) {
	img := halfOpaque(4, 4, 7) // 불투명 8픽셀 = 24비트 < 헤더 32비트
	if _, err := EmbedSpread(img, nil); !errors.Is(err, ErrCapacity) {
		t.Fatalf("want ErrCapacity, got %v", err)
	}
}

func TestTrace(t *testing.T) {
	img := randomImage(20, 20, 4)
	payload := []byte("https://dseung001.github.io/")
	wm, err := Embed(img, payload)
	if err != nil {
		t.Fatal(err)
	}
	steps := Trace(img, wm, len(payload))
	if len(steps) != (headerBytes+len(payload))*8 {
		t.Fatalf("got %d steps", len(steps))
	}
	data := make([]byte, headerBytes+len(payload))
	for i, s := range steps {
		data[i/8] = data[i/8]<<1 | s.Bit
		if s.After&^1 != s.Before&^1 {
			t.Fatalf("step %d: upper bits changed %d→%d", i, s.Before, s.After)
		}
	}
	if !bytes.Equal(data[headerBytes:], payload) {
		t.Fatalf("trace bits decode to %q", data[headerBytes:])
	}
	last := steps[len(steps)-1]
	if last.Section != "payload" || last.ByteIdx != len(payload)-1 {
		t.Fatalf("last step = %+v", last)
	}
}
