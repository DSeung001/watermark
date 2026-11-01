package attack

import (
	"image"
	"math"
	"math/rand/v2"
	"testing"

	"watermark/internal/metrics"
)

// 부드러운 그라디언트에 잡음을 얹은 이미지. 일부 픽셀은 투명하다.
func testImage(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	r := rand.New(rand.NewPCG(1, 2))
	for y := range h {
		for x := range w {
			p := img.Pix[y*img.Stride+x*4:]
			base := 128 + 80*math.Sin(float64(x)/9)*math.Cos(float64(y)/7)
			for c := range 3 {
				p[c] = uint8(base + r.Float64()*20 - 10)
			}
			p[3] = 255
			if x < 4 {
				p[3] = 0
			}
		}
	}
	return img
}

func TestAllCasesKeepSizeAndOpaque(t *testing.T) {
	img := testImage(64, 48)
	for _, c := range Cases {
		res, err := Apply(img, c)
		if err != nil {
			t.Fatalf("%v: %v", c, err)
		}
		if res.Image.Rect != img.Rect {
			t.Fatalf("%v: size %v", c, res.Image.Rect)
		}
		for i := 3; i < len(res.Image.Pix); i += 4 {
			if res.Image.Pix[i] != 255 {
				t.Fatalf("%v: alpha %d at %d", c, res.Image.Pix[i], i)
			}
		}
	}
}

func TestDamageGrowsWithStrength(t *testing.T) {
	img := Opaque(testImage(128, 96))
	mse := func(c Case) float64 {
		res, err := Apply(img, c)
		if err != nil {
			t.Fatal(err)
		}
		return metrics.MSE(img, res.Image)
	}
	for i := 0; i+1 < len(Cases); i++ {
		a, b := Cases[i], Cases[i+1]
		if a.Kind == b.Kind && mse(a) >= mse(b) {
			t.Fatalf("%v (%.2f) should damage less than %v (%.2f)", a, mse(a), b, mse(b))
		}
	}
}

func TestResizeSmallSize(t *testing.T) {
	res, err := Apply(testImage(800, 665), Case{Kind: "resize", Level: 75})
	if err != nil {
		t.Fatal(err)
	}
	if res.SmallW != 600 || res.SmallH != 499 {
		t.Fatalf("small = %dx%d", res.SmallW, res.SmallH)
	}
}

func TestFlatImageSurvivesBlurAndResize(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	want := Opaque(img)
	for _, got := range []*image.NRGBA{Blur(img, 2), first(Resize(img, 0.5))} {
		if metrics.MSE(want, got) != 0 {
			t.Fatal("flat image changed")
		}
	}
}

func first(a, _ *image.NRGBA) *image.NRGBA { return a }

func TestUnknownKind(t *testing.T) {
	if _, err := Apply(testImage(16, 16), Case{Kind: "nope"}); err == nil {
		t.Fatal("want error")
	}
}
