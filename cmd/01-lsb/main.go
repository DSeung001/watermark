// 1편: LSB 치환 워터마크 시뮬레이션.
//
//	go run ./cmd/01-lsb -msg "hello watermark"
package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"log"
	"path/filepath"

	"watermark/internal/imageio"
	"watermark/internal/lsb"
	"watermark/internal/metrics"
	"watermark/internal/visualize"
)

func main() {
	in := flag.String("in", "assets/github_01.png", "원본 PNG 경로")
	msg := flag.String("msg", "https://dseung001.github.io/", "숨길 메시지")
	outDir := flag.String("out", "out/01", "결과 저장 디렉터리")
	flag.Parse()

	orig, err := imageio.Load(*in)
	if err != nil {
		log.Fatal(err)
	}
	b := orig.Bounds()
	fmt.Printf("[원본] %s  %dx%d  용량 %d bytes\n", *in, b.Dx(), b.Dy(), lsb.Capacity(orig))

	payload := []byte(*msg)
	wm, err := lsb.Embed(orig, payload)
	if err != nil {
		log.Fatal(err)
	}
	wmPath := filepath.Join(*outDir, "watermarked.png")
	if err := imageio.Save(wmPath, wm); err != nil {
		log.Fatal(err)
	}
	usedBits := (len(payload) + 4) * 8
	fmt.Printf("[삽입] %q (%d bytes, %d bits → %d 픽셀) → %s\n",
		*msg, len(payload), usedBits, (usedBits+2)/3, wmPath)

	// 메모리상의 이미지가 아니라 저장된 파일을 다시 읽어서 추출한다.
	reloaded, err := imageio.Load(wmPath)
	if err != nil {
		log.Fatal(err)
	}
	got, err := lsb.Extract(reloaded)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("[추출] %q  일치=%v  BER=%.4f\n", got, bytes.Equal(got, payload), metrics.BER(payload, got))

	fmt.Printf("[품질] MSE=%.6f  PSNR=%.2f dB  SSIM=%.7f\n",
		metrics.MSE(orig, reloaded), metrics.PSNR(orig, reloaded), metrics.SSIM(orig, reloaded))

	visuals := map[string]*image.NRGBA{
		"lsb_plane_original.png":    visualize.BitPlane(orig, 0),
		"lsb_plane_watermarked.png": visualize.BitPlane(reloaded, 0),
		"diff_x255.png":             visualize.AmplifiedDiff(orig, reloaded, 255),
	}
	for name, img := range visuals {
		if err := imageio.Save(filepath.Join(*outDir, name), img); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Printf("[시각화] %s/{lsb_plane_original,lsb_plane_watermarked,diff_x255}.png\n", *outDir)
}
