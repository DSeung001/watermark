package ecc

import (
	"bytes"
	"errors"
	"fmt"
)

const (
	// RSMaxLen은 GF(256) Reed-Solomon 코드워드의 최대 바이트 수다.
	RSMaxLen = 255
	// RSPrimitive는 GF(256)의 원시 다항식 x^8+x^4+x^3+x^2+1이다.
	RSPrimitive = 0x11d
)

// ErrUncorrectable은 오류가 정정 한도를 넘었을 때 돌려준다.
var ErrUncorrectable = errors.New("ecc: too many errors")

// GF(256) 지수표와 로그표. 생성원은 α=2다.
var gfExp, gfLog = func() (exp [510]byte, log [256]int) {
	x := 1
	for i := range 255 {
		exp[i], exp[i+255] = byte(x), byte(x)
		log[x] = i
		if x <<= 1; x&0x100 != 0 {
			x ^= RSPrimitive
		}
	}
	return exp, log
}()

func gfMul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return gfExp[gfLog[a]+gfLog[b]]
}

func gfDiv(a, b byte) byte {
	if a == 0 {
		return 0
	}
	return gfExp[gfLog[a]+255-gfLog[b]]
}

// polyEval은 낮은 차수부터 저장한 다항식 값을 Horner 방식으로 구한다.
func polyEval(p []byte, x byte) byte {
	var y byte
	for i := len(p) - 1; i >= 0; i-- {
		y = gfMul(y, x) ^ p[i]
	}
	return y
}

// rsGenerator는 근이 α^0 … α^(nsym-1)인 생성 다항식이다. 높은 차수부터 저장한다.
func rsGenerator(nsym int) []byte {
	g := []byte{1}
	for i := range nsym {
		next := make([]byte, len(g)+1)
		for j, c := range g {
			next[j] ^= c
			next[j+1] ^= gfMul(c, gfExp[i])
		}
		g = next
	}
	return g
}

// RSEncode는 msg 뒤에 패리티 nsym바이트를 붙인 체계적 코드워드를 돌려준다.
func RSEncode(msg []byte, nsym int) ([]byte, error) {
	if len(msg)+nsym > RSMaxLen {
		return nil, fmt.Errorf("ecc: codeword %d exceeds %d bytes", len(msg)+nsym, RSMaxLen)
	}
	g := rsGenerator(nsym)
	out := make([]byte, len(msg)+nsym)
	copy(out, msg)
	// msg·x^nsym을 g로 나눈 나머지가 패리티다.
	for i := range msg {
		c := out[i]
		if c == 0 {
			continue
		}
		for j := 1; j < len(g); j++ {
			out[i+j] ^= gfMul(c, g[j])
		}
	}
	copy(out, msg)
	return out, nil
}

// rsSyndromes는 S_j = c(α^j)다. 오류가 없으면 모두 0이다.
func rsSyndromes(code []byte, nsym int) (syn []byte, clean bool) {
	syn = make([]byte, nsym)
	clean = true
	for j := range nsym {
		var y byte
		for _, c := range code {
			y = gfMul(y, gfExp[j]) ^ c
		}
		syn[j] = y
		clean = clean && y == 0
	}
	return syn, clean
}

// berlekampMassey는 신드롬을 만족하는 가장 짧은 오류 위치 다항식 Λ를 구한다.
func berlekampMassey(syn []byte) []byte {
	loc, prev := []byte{1}, []byte{1}
	errs, shift := 0, 1
	var prevDelta byte = 1
	for n := range syn {
		delta := syn[n]
		for i := 1; i <= errs && i < len(loc); i++ {
			delta ^= gfMul(loc[i], syn[n-i])
		}
		if delta == 0 {
			shift++
			continue
		}
		scale := gfDiv(delta, prevDelta)
		next := make([]byte, max(len(loc), len(prev)+shift))
		copy(next, loc)
		for i, c := range prev {
			next[i+shift] ^= gfMul(scale, c)
		}
		if 2*errs <= n {
			errs, prev, prevDelta, shift = n+1-errs, loc, delta, 1
		} else {
			shift++
		}
		loc = next
	}
	return loc[:min(errs+1, len(loc))]
}

// RSTrace는 복호 단계별 중간값이다.
type RSTrace struct {
	Received   []byte `json:"received"`   // 수신한 코드워드
	Syndromes  []byte `json:"syndromes"`  // S_j = r(α^j)
	Clean      bool   `json:"clean"`      // 신드롬이 모두 0이다
	Locator    []byte `json:"locator"`    // 오류 위치 다항식 Λ, 낮은 차수부터
	Omega      []byte `json:"omega"`      // 오류 값 다항식 Ω, 낮은 차수부터
	Positions  []int  `json:"positions"`  // 찾은 오류의 코드워드 인덱스, 오름차순
	Magnitudes []byte `json:"magnitudes"` // Positions와 같은 순서의 오류 값
	Corrected  []byte `json:"corrected"`  // 정정한 코드워드
	Recheck    []byte `json:"recheck"`    // 정정 후 다시 계산한 신드롬
	Message    []byte `json:"message"`    // 추출한 메시지
	FailedAt   string `json:"failedAt"`   // 실패 지점: degree, chien, forney, verify
}

// RSDecode는 코드워드를 정정해 메시지를 돌려준다. fixed는 고친 바이트 수다.
// 정정에 실패하면 코드워드 앞부분을 그대로 돌려주고 ErrUncorrectable을 함께 준다.
func RSDecode(code []byte, nsym int) (msg []byte, fixed int, err error) {
	t, err := RSDecodeTrace(code, nsym)
	if err != nil {
		return t.Message, 0, err
	}
	return t.Message, len(t.Positions), nil
}

// RSDecodeTrace는 RSDecode와 같게 복호하면서 단계별 중간값을 기록한다.
func RSDecodeTrace(code []byte, nsym int) (RSTrace, error) {
	k := len(code) - nsym
	if k < 0 || len(code) > RSMaxLen {
		return RSTrace{}, fmt.Errorf("ecc: invalid codeword length %d", len(code))
	}
	t := RSTrace{Received: bytes.Clone(code), Message: bytes.Clone(code[:k])}
	fail := func(at string) (RSTrace, error) {
		t.FailedAt = at
		return t, ErrUncorrectable
	}

	t.Syndromes, t.Clean = rsSyndromes(code, nsym)
	if t.Clean {
		t.Corrected, t.Recheck = bytes.Clone(code), t.Syndromes
		return t, nil
	}

	loc := berlekampMassey(t.Syndromes)
	t.Locator = loc
	degree := len(loc) - 1
	if 2*degree > nsym {
		return fail("degree")
	}

	// Chien Search: Λ(X^-1)=0인 X=α^p가 오류 위치다. 인덱스 i의 차수는 p=n-1-i다.
	n := len(code)
	for i := range n {
		if polyEval(loc, gfExp[(255-(n-1-i))%255]) == 0 {
			t.Positions = append(t.Positions, i)
		}
	}
	if len(t.Positions) != degree {
		return fail("chien")
	}

	// Forney: 오류 값 e = X·Ω(X^-1)/Λ'(X^-1)
	omega := make([]byte, nsym)
	for i, s := range t.Syndromes {
		for j, l := range loc {
			if i+j < nsym {
				omega[i+j] ^= gfMul(s, l)
			}
		}
	}
	t.Omega = omega
	deriv := make([]byte, len(loc)-1)
	for i := 1; i < len(loc); i += 2 {
		deriv[i-1] = loc[i]
	}
	out := bytes.Clone(code)
	for _, i := range t.Positions {
		p := n - 1 - i
		inv := gfExp[(255-p)%255]
		d := polyEval(deriv, inv)
		if d == 0 {
			return fail("forney")
		}
		e := gfDiv(gfMul(gfExp[p], polyEval(omega, inv)), d)
		t.Magnitudes = append(t.Magnitudes, e)
		out[i] ^= e
	}

	recheck, ok := rsSyndromes(out, nsym)
	t.Recheck = recheck
	if !ok {
		return fail("verify")
	}
	t.Corrected, t.Message = out, bytes.Clone(out[:k])
	return t, nil
}
