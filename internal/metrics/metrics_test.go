package metrics

import (
	"image"
	"math"
	"math/rand/v2"
	"testing"
)

func randomImage(w, h int, seed uint64) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	r := rand.New(rand.NewPCG(seed, seed))
	for i := range img.Pix {
		img.Pix[i] = uint8(r.UintN(256))
	}
	return img
}

func TestSSIM(t *testing.T) {
	a := randomImage(32, 32, 1)
	if got := SSIM(a, a); math.Abs(got-1) > 1e-9 {
		t.Fatalf("SSIM(a, a) = %v, want 1", got)
	}

	lsbFlip := image.NewNRGBA(a.Rect)
	copy(lsbFlip.Pix, a.Pix)
	for i := range lsbFlip.Pix {
		lsbFlip.Pix[i] ^= 1
	}
	if got := SSIM(a, lsbFlip); got < 0.99 || got >= 1 {
		t.Fatalf("SSIM after LSB flip = %v, want [0.99, 1)", got)
	}

	if got := SSIM(a, randomImage(32, 32, 2)); got > 0.2 {
		t.Fatalf("SSIM of unrelated images = %v, want small", got)
	}
}

func TestBER(t *testing.T) {
	cases := []struct {
		want, got []byte
		ber       float64
	}{
		{nil, nil, 0},
		{[]byte{0xFF}, []byte{0xFF}, 0},
		{[]byte{0xFF}, []byte{0xFE}, 1.0 / 8},
		{[]byte{0x00, 0x00}, []byte{0x00}, 0.5},
	}
	for _, c := range cases {
		if got := BER(c.want, c.got); got != c.ber {
			t.Errorf("BER(%x, %x) = %v, want %v", c.want, c.got, got, c.ber)
		}
	}
}
