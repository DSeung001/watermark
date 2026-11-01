package main

import (
	"encoding/json"
	"net/http"

	"watermark/internal/dct"
	"watermark/internal/ecc"
	"watermark/internal/lsb"
)

// rsParity는 범용 RS(255,223)의 패리티 바이트 수다.
// 메시지가 짧으면 이 패리티를 유지한 채 코드워드만 줄인다.
const rsParity = 32

type rsSettings struct {
	Width       int      `json:"width"`
	Height      int      `json:"height"`
	Strength    float64  `json:"strength"`
	BlockSize   int      `json:"blockSize"`
	Pair        coefPair `json:"pair"`
	MinLuma     int      `json:"minLuma"`
	MaxLuma     int      `json:"maxLuma"`
	HeaderBytes int      `json:"headerBytes"`
	BaseN       int      `json:"baseN"`
	BaseK       int      `json:"baseK"`
	N           int      `json:"n"`
	K           int      `json:"k"`
	Parity      int      `json:"parity"`
	Correctable int      `json:"correctable"`
	Primitive   int      `json:"primitive"`
}

type rsScheme struct {
	Key     string           `json:"key"`
	Name    string           `json:"name"`
	Unit    string           `json:"unit"`
	Slots   int              `json:"slots"`
	Methods []recoveryMethod `json:"methods"` // 다수결, RS 순서
}

type rsResponse struct {
	Message  string     `json:"message"`
	Bits     int        `json:"bits"`
	Sent     []byte     `json:"sent"` // 송신 코드워드
	Settings rsSettings `json:"settings"`
	Schemes  []rsScheme `json:"schemes"`
}

func (s *server) handleRS(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "잘못된 요청: "+err.Error())
		return
	}
	payload, err := codeMessage(req.Message)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	rs, err := rsCodec(payload, rsParity)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	rs.trace = func(voted []byte) *ecc.RSTrace {
		t, _ := ecc.RSDecodeTrace(voted, rsParity) // 실패도 t에 기록된다
		return &t
	}

	b := s.orig.Bounds()
	resp := rsResponse{
		Message: req.Message, Bits: len(payload) * 8, Sent: rs.coded,
		Settings: rsSettings{
			Width: b.Dx(), Height: b.Dy(),
			Strength: dct.DefaultStrength, BlockSize: dct.N,
			Pair:    coefPair{dct.U1, dct.V1, dct.U2, dct.V2},
			MinLuma: dct.MinLuma, MaxLuma: dct.MaxLuma,
			HeaderBytes: lsb.FrameBits(0) / 8,
			BaseN:       ecc.RSMaxLen, BaseK: ecc.RSMaxLen - rsParity,
			N: len(rs.coded), K: len(payload), Parity: rsParity, Correctable: rs.correctable,
			Primitive: ecc.RSPrimitive,
		},
	}
	for _, sc := range []struct{ key, name, unit string }{
		{"lsb", "LSB", "자리"}, {"dct", "DCT", "블록"},
	} {
		out := rsScheme{Key: sc.key, Name: sc.name, Unit: sc.unit}
		for _, c := range []codec{voteCodec(payload), rs} {
			m, err := s.evaluateCodec(c, sc.key, dct.DefaultStrength)
			if err != nil {
				httpError(w, http.StatusBadRequest, err.Error())
				return
			}
			out.Methods = append(out.Methods, m)
		}
		if sc.key == "lsb" {
			out.Slots = len(lsb.Positions(s.orig))
		} else {
			out.Slots = len(dct.Blocks(s.orig))
		}
		resp.Schemes = append(resp.Schemes, out)
	}
	writeJSON(w, resp)
}
