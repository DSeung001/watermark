package dct

import (
	"bytes"
	"errors"
	"image"
	"math"
	"math/rand/v2"
	"path/filepath"
	"testing"

	"watermark/internal/attack"
	"watermark/internal/imageio"
)

// 왼쪽 8칸은 투명 검정, 나머지는 잡음 섞인 회색인 이미지
func testImage(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	r := rand.New(rand.NewPCG(3, 4))
	for y := range h {
		for x := range w {
			if x < N {
				continue
			}
			p := img.Pix[y*img.Stride+x*4:]
			v := uint8(120 + r.IntN(16))
			p[0], p[1], p[2], p[3] = v, v, v, 255
		}
	}
	return img
}

func TestTransformRoundTrip(t *testing.T) {
	var px [N * N]float64
	r := rand.New(rand.NewPCG(1, 1))
	for i := range px {
		px[i] = r.Float64() * 255
	}
	back := Inverse(Forward(px))
	for i := range px {
		if math.Abs(px[i]-back[i]) > 1e-9 {
			t.Fatalf("index %d: %f != %f", i, px[i], back[i])
		}
	}
}

func TestForwardDC(t *testing.T) {
	var px [N * N]float64
	for i := range px {
		px[i] = 100
	}
	co := Forward(px)
	if math.Abs(co[0]-800) > 1e-9 {
		t.Fatalf("DC = %f", co[0])
	}
	for i := 1; i < len(co); i++ {
		if math.Abs(co[i]) > 1e-9 {
			t.Fatalf("AC %d = %f", i, co[i])
		}
	}
}

func TestBlocksSkipTransparentAndDark(t *testing.T) {
	img := testImage(64, 32)
	for _, b := range Blocks(img) {
		if b.X < N {
			t.Fatalf("transparent block selected: %+v", b)
		}
	}
	dark := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for i := 3; i < len(dark.Pix); i += 4 {
		dark.Pix[i] = 255
	}
	if n := len(Blocks(dark)); n != 0 {
		t.Fatalf("%d dark blocks selected", n)
	}
}

func TestRoundTrip(t *testing.T) {
	img := testImage(128, 96)
	payload := []byte("dct!")
	wm, err := Embed(img, payload, DefaultStrength)
	if err != nil {
		t.Fatal(err)
	}
	blocks := Blocks(img)
	tally := Extract(wm, blocks, len(payload))
	if got := tally.Bytes(); !bytes.Equal(got, payload) {
		t.Fatalf("got %q", got)
	}
	if errs, _ := tally.Errors(payload); errs != 0 {
		t.Fatalf("%d block errors without attack", errs)
	}
}

func TestEmbedKeepsTransparentPixels(t *testing.T) {
	img := testImage(64, 64)
	wm, err := Embed(img, []byte{0xA5}, DefaultStrength)
	if err != nil {
		t.Fatal(err)
	}
	for y := range 64 {
		for x := range 64 {
			o := img.PixOffset(x, y)
			if img.Pix[o+3] != wm.Pix[o+3] {
				t.Fatalf("alpha changed at (%d,%d)", x, y)
			}
			if x < N && !bytes.Equal(img.Pix[o:o+4], wm.Pix[o:o+4]) {
				t.Fatalf("transparent pixel changed at (%d,%d)", x, y)
			}
		}
	}
}

func TestCapacityExceeded(t *testing.T) {
	img := testImage(32, 32)
	n := Capacity(Blocks(img)) + 1
	if _, err := Embed(img, make([]byte, n), DefaultStrength); !errors.Is(err, ErrCapacity) {
		t.Fatalf("want ErrCapacity, got %v", err)
	}
}

// 실제 원본에서 가벼운 공격은 다수결로 완전히 복원돼야 한다.
func TestSurvivesMildAttacks(t *testing.T) {
	img, err := imageio.Load(filepath.Join("..", "..", "assets", "github_01.png"))
	if err != nil {
		t.Skip("asset not found:", err)
	}
	payload := []byte("https://dseung001.github.io/")
	wm, err := Embed(img, payload, DefaultStrength)
	if err != nil {
		t.Fatal(err)
	}
	blocks := Blocks(img)
	for _, c := range []attack.Case{{Kind: "jpeg", Level: 90}, {Kind: "blur", Level: 1}, {Kind: "resize", Level: 75}} {
		res, err := attack.Apply(wm, c)
		if err != nil {
			t.Fatal(err)
		}
		if got := Extract(res.Image, blocks, len(payload)).Bytes(); !bytes.Equal(got, payload) {
			t.Fatalf("%+v: got %q", c, got)
		}
	}
}
