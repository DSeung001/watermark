// 1편: LSB 치환 워터마크 웹 시각화.
//
//	go run ./cmd/01-lsb-web   →  http://localhost:8080
package main

import (
	"bytes"
	"embed"
	"encoding/base64"
	"encoding/json"
	"flag"
	"image"
	"image/png"
	"io/fs"
	"log"
	"math"
	"net/http"

	"watermark/internal/imageio"
	"watermark/internal/lsb"
	"watermark/internal/metrics"
)

//go:embed static
var staticFS embed.FS

// 시각화용 비트 기록은 페이로드 앞쪽 일부만 보낸다.
const maxTraceBytes = 64

type embedResponse struct {
	Message      string        `json:"message"`
	Width        int           `json:"width"`
	Height       int           `json:"height"`
	Capacity     int           `json:"capacity"`
	PayloadBytes int           `json:"payloadBytes"`
	UsedBits     int           `json:"usedBits"`
	UsedPixels   int           `json:"usedPixels"`
	Extracted    string        `json:"extracted"`
	Match        bool          `json:"match"`
	BER          float64       `json:"ber"`
	MSE          float64       `json:"mse"`
	PSNR         *float64      `json:"psnr"` // 동일 이미지면 null(+Inf)
	SSIM         float64       `json:"ssim"`
	Watermarked  string        `json:"watermarked"`
	Trace        []lsb.BitStep `json:"trace"`
}

type server struct {
	orig *image.NRGBA
}

func main() {
	addr := flag.String("addr", ":8080", "listen 주소")
	in := flag.String("in", "assets/github_01.png", "원본 PNG 경로")
	flag.Parse()

	orig, err := imageio.Load(*in)
	if err != nil {
		log.Fatal(err)
	}
	s := &server{orig: orig}

	static, _ := fs.Sub(staticFS, "static")
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(static))
	mux.HandleFunc("GET /img/original.png", s.handleOriginal)
	mux.HandleFunc("GET /img/attacked.png", s.handleAttackedImage)
	mux.HandleFunc("POST /api/embed", s.handleEmbed)
	mux.HandleFunc("POST /api/attack", s.handleAttack)
	mux.HandleFunc("POST /api/dct/embed", s.handleDCTEmbed)
	mux.HandleFunc("POST /api/recovery", s.handleRecovery)

	log.Printf("listening on http://localhost%s (원본: %s)", *addr, *in)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func (s *server) handleOriginal(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/png")
	png.Encode(w, s.orig)
}

func (s *server) handleEmbed(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "잘못된 요청: "+err.Error())
		return
	}
	payload := []byte(req.Message)

	wm, err := lsb.Embed(s.orig, payload)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 파일로 저장한 것과 같은 조건을 만들기 위해 PNG 인코드→디코드 후 추출한다.
	var buf bytes.Buffer
	if err := png.Encode(&buf, wm); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	wmPNG := buf.Bytes()
	reloaded, err := imageio.Decode(bytes.NewReader(wmPNG))
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	got, err := lsb.Extract(reloaded)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}

	usedBits := (len(payload) + 4) * 8
	b := s.orig.Bounds()
	resp := embedResponse{
		Message:      req.Message,
		Width:        b.Dx(),
		Height:       b.Dy(),
		Capacity:     lsb.Capacity(s.orig),
		PayloadBytes: len(payload),
		UsedBits:     usedBits,
		UsedPixels:   (usedBits + 2) / 3,
		Extracted:    string(got),
		Match:        bytes.Equal(got, payload),
		BER:          metrics.BER(payload, got),
		MSE:          metrics.MSE(s.orig, reloaded),
		SSIM:         metrics.SSIM(s.orig, reloaded),
		Watermarked:  dataURL(wmPNG),
		Trace:        lsb.Trace(s.orig, reloaded, min(len(payload), maxTraceBytes)),
	}
	if p := metrics.PSNR(s.orig, reloaded); !math.IsInf(p, 1) {
		resp.PSNR = &p
	}
	writeJSON(w, resp)
}

func dataURL(pngBytes []byte) string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
