package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"

	"watermark/internal/attack"
	"watermark/internal/dct"
	"watermark/internal/ecc"
	"watermark/internal/metrics"
)

const maxCodeMessageBytes = 64 // Hamming과 RS가 블록 용량 안에 들어가는 메시지 상한

// codec은 메시지를 블록에 넣기 전의 부호화와 읽은 뒤의 복호를 묶는다.
// 삽입은 모두 dct.Embed가 맡아 coded를 블록 전체에 순환 반복한다.
type codec struct {
	key, name   string
	coded       []byte
	parity      int // RS만: 패리티 바이트 수
	correctable int // RS만: 고칠 수 있는 바이트 수
	decode      func(voted []byte) (msg []byte, fixed int, failed bool)
}

func (s *server) codecs(payload []byte) ([]codec, error) {
	hamming := ecc.Encode74(payload)

	// RS 코드워드는 블록 전체를 한 번만 쓰도록 용량에 맞춰 줄인다.
	n := min(ecc.RSMaxLen, dct.Capacity(dct.Blocks(s.orig)))
	nsym := n - len(payload)
	if nsym < 2 {
		return nil, fmt.Errorf("%w: %d bytes", dct.ErrCapacity, len(payload))
	}
	rs, err := ecc.RSEncode(payload, nsym)
	if err != nil {
		return nil, err
	}

	return []codec{
		{
			key: "vote", name: "다수결", coded: payload,
			decode: func(voted []byte) ([]byte, int, bool) { return voted, 0, false },
		},
		{
			key: "hamming", name: "Hamming(7,4)", coded: hamming,
			decode: func(voted []byte) ([]byte, int, bool) {
				msg, fixed := ecc.Decode74(voted, len(payload))
				return msg, fixed, false
			},
		},
		{
			key: "rs", name: fmt.Sprintf("RS(%d,%d)", n, len(payload)), coded: rs,
			parity: nsym, correctable: nsym / 2,
			decode: func(voted []byte) ([]byte, int, bool) {
				msg, fixed, err := ecc.RSDecode(voted, nsym)
				return msg, fixed, errors.Is(err, ecc.ErrUncorrectable)
			},
		},
	}, nil
}

type recoveryCase struct {
	attack.Case
	CodedBER    float64 `json:"codedBer"`    // 복사본 하나하나를 본 부호 비트 오류율
	CodedErrors int     `json:"codedErrors"` // 틀린 자리 수
	CodedTotal  int     `json:"codedTotal"`  // 전체 자리 수
	VotedErrors int     `json:"votedErrors"` // 다수결 후 남은 부호 비트 오류 수
	Fixed       int     `json:"fixed"`       // 정정한 코드워드(Hamming) 또는 바이트(RS) 수
	Failed      bool    `json:"failed"`      // RS만: 정정 한도 초과
	BER         float64 `json:"ber"`         // 복구한 메시지의 BER
	Errors      int     `json:"errors"`
	Bits        int     `json:"bits"`
	Extracted   string  `json:"extracted"`
	Match       bool    `json:"match"`
}

type recoveryMethod struct {
	Key         string         `json:"key"`
	Name        string         `json:"name"`
	CodedBits   int            `json:"codedBits"`
	Repeat      float64        `json:"repeat"` // 코드 전체가 반복된 횟수
	Parity      int            `json:"parity"`
	Correctable int            `json:"correctable"`
	PSNR        *float64       `json:"psnr"`
	Cases       []recoveryCase `json:"cases"`
}

type recoveryResponse struct {
	Message  string           `json:"message"`
	Strength float64          `json:"strength"`
	Blocks   int              `json:"blocks"`
	Bits     int              `json:"bits"`
	Methods  []recoveryMethod `json:"methods"`
}

func (s *server) evaluateCodec(c codec, strength float64) (recoveryMethod, error) {
	sc, err := s.build("dct", string(c.coded), strength)
	if err != nil {
		return recoveryMethod{}, err
	}
	bits := len(c.coded) * 8
	out := recoveryMethod{
		Key: c.key, Name: c.name, CodedBits: bits, Repeat: float64(sc.slots) / float64(bits),
		Parity: c.parity, Correctable: c.correctable,
		PSNR: finite(metrics.PSNR(s.orig, sc.raw)),
	}
	for _, ac := range attack.Cases {
		res, err := attack.Apply(sc.wm, ac)
		if err != nil {
			return recoveryMethod{}, err
		}
		t := sc.read(res.Image)
		voted := t.Bytes()
		codedErrs, codedTotal := t.Errors(c.coded)
		msg, fixed, failed := c.decode(voted)
		payload := c.message()
		ber := metrics.BER(payload, msg)
		out.Cases = append(out.Cases, recoveryCase{
			Case: ac, CodedBER: float64(codedErrs) / float64(codedTotal),
			CodedErrors: codedErrs, CodedTotal: codedTotal,
			VotedErrors: int(math.Round(metrics.BER(c.coded, voted) * float64(bits))),
			Fixed:       fixed, Failed: failed,
			BER: ber, Errors: int(math.Round(ber * float64(len(payload)*8))), Bits: len(payload) * 8,
			Extracted: strings.ToValidUTF8(string(msg), "�"),
			Match:     bytes.Equal(msg, payload),
		})
	}
	return out, nil
}

// message는 오류 없는 코드를 복호해 원래 메시지를 얻는다. 응답의 정답 비교에만 쓴다.
func (c codec) message() []byte {
	msg, _, _ := c.decode(c.coded)
	return msg
}

func (s *server) handleRecovery(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Message  string  `json:"message"`
		Strength float64 `json:"strength"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "잘못된 요청: "+err.Error())
		return
	}
	payload := []byte(req.Message)
	if len(payload) == 0 {
		httpError(w, http.StatusBadRequest, "메시지를 입력하세요")
		return
	}
	if len(payload) > maxCodeMessageBytes {
		httpError(w, http.StatusBadRequest, fmt.Sprintf("메시지는 %d바이트까지 가능합니다", maxCodeMessageBytes))
		return
	}
	codecs, err := s.codecs(payload)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp := recoveryResponse{
		Message: req.Message, Strength: req.Strength,
		Blocks: len(dct.Blocks(s.orig)), Bits: len(payload) * 8,
	}
	for _, c := range codecs {
		m, err := s.evaluateCodec(c, req.Strength)
		if err != nil {
			httpError(w, http.StatusBadRequest, err.Error())
			return
		}
		resp.Methods = append(resp.Methods, m)
	}
	writeJSON(w, resp)
}
