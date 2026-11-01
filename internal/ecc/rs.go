package ecc

import (
	"errors"
	"fmt"
)

// RSMaxLen은 GF(256) Reed-Solomon 코드워드의 최대 바이트 수다.
const RSMaxLen = 255

// ErrUncorrectable은 오류가 정정 한도를 넘었을 때 돌려준다.
var ErrUncorrectable = errors.New("ecc: too many errors")

// GF(256) 지수표와 로그표. 원시 다항식은 x^8+x^4+x^3+x^2+1(0x11d)이다.
var gfExp, gfLog = func() (exp [510]byte, log [256]int) {
	x := 1
	for i := range 255 {
		exp[i], exp[i+255] = byte(x), byte(x)
		log[x] = i
		if x <<= 1; x&0x100 != 0 {
			x ^= 0x11d
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

// RSDecode는 코드워드를 정정해 메시지를 돌려준다. fixed는 고친 바이트 수다.
// 정정에 실패하면 코드워드 앞부분을 그대로 돌려주고 ErrUncorrectable을 함께 준다.
func RSDecode(code []byte, nsym int) (msg []byte, fixed int, err error) {
	k := len(code) - nsym
	if k < 0 || len(code) > RSMaxLen {
		return nil, 0, fmt.Errorf("ecc: invalid codeword length %d", len(code))
	}
	received := code[:k:k]
	syn, clean := rsSyndromes(code, nsym)
	if clean {
		return append([]byte(nil), received...), 0, nil
	}

	loc := berlekampMassey(syn)
	degree := len(loc) - 1
	if 2*degree > nsym {
		return append([]byte(nil), received...), 0, ErrUncorrectable
	}

	// Chien Search: Λ(X^-1)=0인 X=α^p가 오류 위치다.
	n := len(code)
	var powers []int
	for p := range n {
		if polyEval(loc, gfExp[(255-p)%255]) == 0 {
			powers = append(powers, p)
		}
	}
	if len(powers) != degree {
		return append([]byte(nil), received...), 0, ErrUncorrectable
	}

	// Forney: 오류 값 e = X·Ω(X^-1)/Λ'(X^-1)
	omega := make([]byte, nsym)
	for i, s := range syn {
		for j, l := range loc {
			if i+j < nsym {
				omega[i+j] ^= gfMul(s, l)
			}
		}
	}
	deriv := make([]byte, len(loc)-1)
	for i := 1; i < len(loc); i += 2 {
		deriv[i-1] = loc[i]
	}
	out := append([]byte(nil), code...)
	for _, p := range powers {
		inv := gfExp[(255-p)%255]
		d := polyEval(deriv, inv)
		if d == 0 {
			return append([]byte(nil), received...), 0, ErrUncorrectable
		}
		out[n-1-p] ^= gfDiv(gfMul(gfExp[p], polyEval(omega, inv)), d)
	}
	if _, ok := rsSyndromes(out, nsym); !ok {
		return append([]byte(nil), received...), 0, ErrUncorrectable
	}
	return out[:k:k], degree, nil
}
