package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"net/http"
	"strconv"
	"strings"

	"watermark/internal/attack"
	"watermark/internal/dct"
	"watermark/internal/imageio"
	"watermark/internal/lsb"
	"watermark/internal/metrics"
	"watermark/internal/visualize"
	"watermark/internal/vote"
)

const (
	maxMessageBytes = 1024
	maxShowBits     = 512 // 응답에 넣는 비트 격자 상한
	sampleCount     = 10  // 값 변화 표의 행 수
	diffGain        = 32  // 차이 이미지 증폭 배율
	embedDiffGain   = 10  // 삽입 변화량 증폭 배율. 강도 96에서도 포화되지 않는 값
	minStrength     = 4
	maxStrength     = 96
)

// sample은 삽입 자리 하나의 공격 전후 값이다.
// LSB는 채널 값, DCT는 계수 차이 c1-c2다.
type sample struct {
	Loc    string  `json:"loc"`
	Before float64 `json:"before"`
	After  float64 `json:"after"`
	Want   uint8   `json:"want"`
	Got    uint8   `json:"got"`
}

type attackCase struct {
	attack.Case
	BER       float64  `json:"ber"`       // 다수결
	Errors    int      `json:"errors"`    // 다수결 오류 비트 수
	Bits      int      `json:"bits"`      // 메시지 비트 수
	RawBER    float64  `json:"rawBer"`    // 삽입 자리 단위
	RawErrors int      `json:"rawErrors"` // 틀린 자리 수
	RawTotal  int      `json:"rawTotal"`  // 전체 자리 수
	PSNR      *float64 `json:"psnr"`
	SSIM      float64  `json:"ssim"`
	HeaderOK  *bool    `json:"headerOk"` // LSB만: 첫 프레임 길이 헤더가 맞는가
	Header    *uint32  `json:"header"`
	Extracted string   `json:"extracted"`
	Match     bool     `json:"match"`
	WantBits  string   `json:"wantBits"`
	GotBits   string   `json:"gotBits"`
	Ones      []int    `json:"ones"`
	Copies    []int    `json:"copies"`
	SmallW    int      `json:"smallW,omitempty"`
	SmallH    int      `json:"smallH,omitempty"`
	JPEGBytes int      `json:"jpegBytes,omitempty"`
	Samples   []sample `json:"samples"`
}

type attackResponse struct {
	Method    string       `json:"method"`
	Message   string       `json:"message"`
	Strength  float64      `json:"strength"`
	Width     int          `json:"width"`
	Height    int          `json:"height"`
	Slots     int          `json:"slots"`  // 삽입 자리 수
	Repeat    int          `json:"repeat"` // 메시지 전체가 반복된 횟수
	Unit      string       `json:"unit"`
	DiffGain  int          `json:"diffGain"`
	EmbedGain int          `json:"embedGain"`
	WmPSNR    *float64     `json:"wmPsnr"` // 원본 대비 삽입 품질
	WmSSIM    float64      `json:"wmSsim"`
	Cases     []attackCase `json:"cases"`
}

// scheme은 한 방식으로 워터마크를 넣은 결과와 읽는 방법을 묶는다.
type scheme struct {
	method  string
	unit    string
	payload []byte
	raw     *image.NRGBA // 삽입 직후 (alpha 유지)
	wm      *image.NRGBA // 공격 전 기준 이미지 (불투명)
	slots   int
	frame   int // 한 번 반복되는 비트 수. LSB는 길이 헤더를 포함한다
	repeat  int
	read    func(*image.NRGBA) vote.Tally
	header  func(*image.NRGBA) uint32 // LSB만
	samples func(*image.NRGBA) []sample
	errMap  func(*image.NRGBA) *image.NRGBA
}

func (s *server) build(method, message string, strength float64) (*scheme, error) {
	payload := []byte(message)
	switch {
	case len(payload) == 0:
		return nil, errors.New("메시지를 입력하세요")
	case len(payload) > maxMessageBytes:
		return nil, fmt.Errorf("메시지는 %d바이트까지 가능합니다", maxMessageBytes)
	}
	switch method {
	case "lsb":
		return s.buildLSB(payload)
	case "dct":
		if strength == 0 {
			strength = dct.DefaultStrength
		}
		if strength < minStrength || strength > maxStrength {
			return nil, fmt.Errorf("강도는 %d~%d 범위입니다", minStrength, maxStrength)
		}
		return s.buildDCT(payload, strength)
	}
	return nil, fmt.Errorf("알 수 없는 방식: %q", method)
}

func (s *server) buildLSB(payload []byte) (*scheme, error) {
	raw, err := lsb.EmbedSpread(s.orig, payload)
	if err != nil {
		return nil, err
	}
	pos := lsb.Positions(s.orig)
	frame := lsb.Frame(payload)
	frameBits := len(frame) * 8
	w := s.orig.Rect.Dx()
	wm := attack.Opaque(raw)

	// bit: 해당 자리에 넣은 프레임 비트
	bit := func(i int) uint8 { return vote.Bit(frame, i%frameBits) }

	return &scheme{
		method: "lsb", unit: "자리", payload: payload, raw: raw, wm: wm,
		slots: len(pos), frame: frameBits, repeat: len(pos) / frameBits,
		read:   func(img *image.NRGBA) vote.Tally { return lsb.ExtractSpread(img, pos, len(payload)) },
		header: func(img *image.NRGBA) uint32 { return lsb.SpreadHeader(img, pos) },
		samples: func(img *image.NRGBA) []sample {
			out := make([]sample, 0, sampleCount)
			for k := range sampleCount {
				i := (k*2 + 1) * len(pos) / (sampleCount * 2)
				// 메시지 비트에 해당하는 자리로 옮긴다.
				for i%frameBits < 32 {
					i++
				}
				idx := pos[i]
				p := idx / 4
				out = append(out, sample{
					Loc:    fmt.Sprintf("(%d,%d) %c", p%w, p/w, "RGB"[idx%4]),
					Before: float64(wm.Pix[idx]),
					After:  float64(img.Pix[idx]),
					Want:   bit(i),
					Got:    img.Pix[idx] & 1,
				})
			}
			return out
		},
		errMap: func(img *image.NRGBA) *image.NRGBA {
			out := blankMap(s.orig)
			wrong := make([]uint8, w*s.orig.Rect.Dy())
			for i, idx := range pos {
				if img.Pix[idx]&1 != bit(i) {
					wrong[idx/4]++
				}
			}
			for i := 0; i < len(pos); i += 3 {
				p := pos[i] / 4
				out.SetNRGBA(p%w, p/w, errColor(float64(wrong[p])/3))
			}
			return out
		},
	}, nil
}

func (s *server) buildDCT(payload []byte, strength float64) (*scheme, error) {
	raw, err := dct.Embed(s.orig, payload, strength)
	if err != nil {
		return nil, err
	}
	blocks := dct.Blocks(s.orig)
	nBits := len(payload) * 8
	wm := attack.Opaque(raw)
	bit := func(k int) uint8 { return vote.Bit(payload, k%nBits) }

	return &scheme{
		method: "dct", unit: "블록", payload: payload, raw: raw, wm: wm,
		slots: len(blocks), frame: nBits, repeat: len(blocks) / nBits,
		read: func(img *image.NRGBA) vote.Tally { return dct.Extract(img, blocks, len(payload)) },
		samples: func(img *image.NRGBA) []sample {
			out := make([]sample, 0, sampleCount)
			for i := range sampleCount {
				k := (i*2 + 1) * len(blocks) / (sampleCount * 2)
				b := blocks[k]
				out = append(out, sample{
					Loc:    fmt.Sprintf("블록 (%d,%d)", b.X, b.Y),
					Before: dct.Diff(wm, b),
					After:  dct.Diff(img, b),
					Want:   bit(k),
					Got:    dct.ReadBit(img, b),
				})
			}
			return out
		},
		errMap: func(img *image.NRGBA) *image.NRGBA {
			out := blankMap(s.orig)
			for k, b := range blocks {
				rate := 0.0
				if dct.ReadBit(img, b) != bit(k) {
					rate = 1
				}
				for y := b.Y; y < b.Y+dct.N; y++ {
					for x := b.X; x < b.X+dct.N; x++ {
						out.SetNRGBA(x, y, errColor(rate))
					}
				}
			}
			return out
		},
	}, nil
}

// blankMap은 오류 지도의 바탕이다. 삽입에 쓰지 않은 곳은 어두운 회색으로 둔다.
func blankMap(ref *image.NRGBA) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, ref.Rect.Dx(), ref.Rect.Dy()))
	for i := 0; i < len(out.Pix); i += 4 {
		out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = 38, 38, 36, 255
	}
	return out
}

// errColor는 오류 비율(0~1)을 밝은 회색→빨강으로 칠한다.
func errColor(rate float64) color.NRGBA {
	mix := func(a, b float64) uint8 { return uint8(a + (b-a)*rate) }
	return color.NRGBA{mix(224, 224), mix(222, 49), mix(214, 49), 255}
}

func (sc *scheme) evaluate(c attack.Case, res attack.Result) attackCase {
	t := sc.read(res.Image)
	got := t.Bytes()
	rawErrs, rawTotal := t.Errors(sc.payload)
	nBits := len(sc.payload) * 8
	errs := int(math.Round(metrics.BER(sc.payload, got) * float64(nBits)))
	show := min(nBits, maxShowBits)

	var want, gotBits strings.Builder
	for i := range show {
		want.WriteByte('0' + vote.Bit(sc.payload, i))
		gotBits.WriteByte('0' + vote.Bit(got, i))
	}
	out := attackCase{
		Case: c, BER: metrics.BER(sc.payload, got), Errors: errs, Bits: nBits,
		RawBER: float64(rawErrs) / float64(rawTotal), RawErrors: rawErrs, RawTotal: rawTotal,
		SSIM:      metrics.SSIM(sc.wm, res.Image),
		Extracted: strings.ToValidUTF8(string(got), "�"),
		Match:     bytes.Equal(got, sc.payload),
		WantBits:  want.String(), GotBits: gotBits.String(),
		Ones: t.Ones[:show], Copies: t.Copies[:show],
		SmallW: res.SmallW, SmallH: res.SmallH, JPEGBytes: res.JPEGBytes,
		Samples: sc.samples(res.Image),
	}
	out.PSNR = finite(metrics.PSNR(sc.wm, res.Image))
	if sc.header != nil {
		h := sc.header(res.Image)
		ok := h == uint32(len(sc.payload))
		out.Header, out.HeaderOK = &h, &ok
	}
	return out
}

func finite(v float64) *float64 {
	if math.IsInf(v, 1) {
		return nil
	}
	return &v
}

func (s *server) handleAttack(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Message  string  `json:"message"`
		Method   string  `json:"method"`
		Strength float64 `json:"strength"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "잘못된 요청: "+err.Error())
		return
	}
	sc, err := s.build(req.Method, req.Message, req.Strength)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}

	b := s.orig.Bounds()
	resp := attackResponse{
		Method: sc.method, Message: req.Message, Strength: req.Strength,
		Width: b.Dx(), Height: b.Dy(),
		Slots: sc.slots, Repeat: sc.repeat, Unit: sc.unit, DiffGain: diffGain, EmbedGain: embedDiffGain,
		WmPSNR: finite(metrics.PSNR(s.orig, sc.raw)),
		WmSSIM: metrics.SSIM(s.orig, sc.raw),
	}
	for _, c := range attack.Cases {
		res, err := attack.Apply(sc.wm, c)
		if err != nil {
			httpError(w, http.StatusInternalServerError, err.Error())
			return
		}
		resp.Cases = append(resp.Cases, sc.evaluate(c, res))
	}
	writeJSON(w, resp)
}

// handleAttackedImage는 조건 하나의 결과 이미지를 돌려준다.
// 같은 입력이면 결과가 같으므로 서버에 상태를 두지 않고 매번 계산한다.
func (s *server) handleAttackedImage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	strength, _ := strconv.ParseFloat(q.Get("strength"), 64)
	sc, err := s.build(q.Get("method"), q.Get("message"), strength)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}

	img := sc.wm
	if kind := q.Get("attack"); kind != "none" {
		level, _ := strconv.Atoi(q.Get("level"))
		c, ok := findCase(kind, level)
		if !ok {
			httpError(w, http.StatusBadRequest, "알 수 없는 공격 조건")
			return
		}
		res, err := attack.Apply(sc.wm, c)
		if err != nil {
			httpError(w, http.StatusInternalServerError, err.Error())
			return
		}
		img = res.Image
	}

	var out *image.NRGBA
	switch q.Get("view") {
	case "attacked":
		out = img
	case "diff":
		out = visualize.AmplifiedDiff(sc.wm, img, diffGain)
	case "embed":
		out = visualize.AmplifiedDiff(s.orig, sc.raw, embedDiffGain)
	case "raw":
		out = sc.raw
	case "plane":
		out = visualize.BitPlane(img, 0)
	case "errors":
		out = sc.errMap(img)
	default:
		httpError(w, http.StatusBadRequest, "알 수 없는 view")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	png.Encode(w, out)
}

func findCase(kind string, level int) (attack.Case, bool) {
	for _, c := range attack.Cases {
		if c.Kind == kind && c.Level == level {
			return c, true
		}
	}
	return attack.Case{}, false
}

type coefPair struct {
	U1, V1, U2, V2 int
}

type dctTrace struct {
	Block      dct.Block              `json:"block"`
	Index      int                    `json:"index"`
	Bit        uint8                  `json:"bit"`
	Before     [dct.N * dct.N]float64 `json:"before"`
	After      [dct.N * dct.N]float64 `json:"after"`
	DiffBefore float64                `json:"diffBefore"`
	DiffAfter  float64                `json:"diffAfter"`
}

type dctResponse struct {
	Message      string    `json:"message"`
	Width        int       `json:"width"`
	Height       int       `json:"height"`
	Capacity     int       `json:"capacity"`
	PayloadBytes int       `json:"payloadBytes"`
	Blocks       int       `json:"blocks"`
	TotalBlocks  int       `json:"totalBlocks"`
	Repeat       int       `json:"repeat"`
	Strength     float64   `json:"strength"`
	Pair         coefPair  `json:"pair"`
	Extracted    string    `json:"extracted"`
	Match        bool      `json:"match"`
	BER          float64   `json:"ber"`
	MSE          float64   `json:"mse"`
	PSNR         *float64  `json:"psnr"`
	SSIM         float64   `json:"ssim"`
	Watermarked  string    `json:"watermarked"`
	BlockMap     string    `json:"blockMap"` // 칸 하나가 블록 하나인 투명 PNG
	Trace        *dctTrace `json:"trace"`
}

func (s *server) handleDCTEmbed(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Message  string  `json:"message"`
		Strength float64 `json:"strength"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "잘못된 요청: "+err.Error())
		return
	}
	sc, err := s.build("dct", req.Message, req.Strength)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	strength := req.Strength
	if strength == 0 {
		strength = dct.DefaultStrength
	}

	// 파일로 저장한 것과 같은 조건을 만들기 위해 PNG 인코드→디코드 후 추출한다.
	var buf bytes.Buffer
	if err := png.Encode(&buf, sc.raw); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	wmPNG := buf.Bytes()
	reloaded, err := imageio.Decode(bytes.NewReader(wmPNG))
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	got := sc.read(reloaded).Bytes()

	blocks := dct.Blocks(s.orig)
	b := s.orig.Bounds()
	cols, rows := b.Dx()/dct.N, b.Dy()/dct.N
	resp := dctResponse{
		Message: req.Message, Width: b.Dx(), Height: b.Dy(),
		Capacity: dct.Capacity(blocks), PayloadBytes: len(sc.payload),
		Blocks: len(blocks), TotalBlocks: cols * rows, Repeat: sc.repeat,
		Strength:    strength,
		Pair:        coefPair{dct.U1, dct.V1, dct.U2, dct.V2},
		Extracted:   strings.ToValidUTF8(string(got), "�"),
		Match:       bytes.Equal(got, sc.payload),
		BER:         metrics.BER(sc.payload, got),
		MSE:         metrics.MSE(s.orig, reloaded),
		PSNR:        finite(metrics.PSNR(s.orig, reloaded)),
		SSIM:        metrics.SSIM(s.orig, reloaded),
		Watermarked: dataURL(wmPNG),
		BlockMap:    blockMapURL(blocks, cols, rows),
		Trace:       traceBlock(s.orig, reloaded, blocks, sc.payload),
	}
	writeJSON(w, resp)
}

// traceBlock은 삽입으로 계수가 실제로 바뀐 첫 블록의 전후를 기록한다.
func traceBlock(orig, wm *image.NRGBA, blocks []dct.Block, payload []byte) *dctTrace {
	nBits := len(payload) * 8
	for k, b := range blocks {
		before, after := dct.Diff(orig, b), dct.Diff(wm, b)
		if math.Abs(before-after) < 0.5 {
			continue
		}
		return &dctTrace{
			Block: b, Index: k, Bit: vote.Bit(payload, k%nBits),
			Before: dct.Coefs(orig, b), After: dct.Coefs(wm, b),
			DiffBefore: before, DiffAfter: after,
		}
	}
	return nil
}

func blockMapURL(blocks []dct.Block, cols, rows int) string {
	img := image.NewNRGBA(image.Rect(0, 0, cols, rows))
	for _, b := range blocks {
		img.SetNRGBA(b.X/dct.N, b.Y/dct.N, color.NRGBA{47, 111, 222, 150})
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return dataURL(buf.Bytes())
}
