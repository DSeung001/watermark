package ecc

import (
	"bytes"
	"errors"
	"math/rand/v2"
	"slices"
	"testing"
)

func TestEncode74Example(t *testing.T) {
	// 데이터 1011은 P1=0 P2=1 D1=1 P4=0 D2=0 D3=1 D4=1, 즉 0110011이 된다.
	got := Encode74([]byte{0xB0})
	want := byte(0b01100110)
	if got[0] != want {
		t.Fatalf("got %08b, want %08b", got[0], want)
	}
}

func TestHamming74Len(t *testing.T) {
	for n, want := range map[int]int{1: 2, 2: 4, 8: 14, 28: 49} {
		if got := Hamming74Len(n); got != want {
			t.Errorf("Hamming74Len(%d) = %d, want %d", n, got, want)
		}
	}
}

func TestHamming74FixesEverySingleBit(t *testing.T) {
	data := []byte("watermark")
	coded := Encode74(data)
	if got, fixed := Decode74(coded, len(data)); !bytes.Equal(got, data) || fixed != 0 {
		t.Fatalf("clean: got %q, fixed %d", got, fixed)
	}
	for i := range len(data) * 14 {
		bad := bytes.Clone(coded)
		bad[i/8] ^= 1 << (7 - i%8)
		got, fixed := Decode74(bad, len(data))
		if !bytes.Equal(got, data) || fixed != 1 {
			t.Fatalf("bit %d: got %q, fixed %d", i, got, fixed)
		}
	}
}

func TestHamming74TwoBitsInOneWordFail(t *testing.T) {
	data := []byte{0xB0}
	bad := Encode74(data)
	bad[0] ^= 0b11000000
	if got, _ := Decode74(bad, 1); bytes.Equal(got, data) {
		t.Fatal("two errors in a codeword must not decode to the original")
	}
}

// corrupt는 서로 다른 n개 위치의 바이트를 다른 값으로 바꾼다.
func corrupt(code []byte, n int, r *rand.Rand) []byte {
	out := bytes.Clone(code)
	for _, i := range r.Perm(len(out))[:n] {
		out[i] ^= byte(1 + r.IntN(255))
	}
	return out
}

func TestRSCorrectsUpToLimit(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for _, tc := range []struct{ k, nsym int }{{1, 2}, {28, 142}, {223, 32}, {64, 106}} {
		msg := make([]byte, tc.k)
		for i := range msg {
			msg[i] = byte(r.IntN(256))
		}
		code, err := RSEncode(msg, tc.nsym)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(code[:tc.k], msg) {
			t.Fatal("code must be systematic")
		}
		for errs := 0; errs <= tc.nsym/2; errs++ {
			got, fixed, err := RSDecode(corrupt(code, errs, r), tc.nsym)
			if err != nil {
				t.Fatalf("RS k=%d nsym=%d errs=%d: %v", tc.k, tc.nsym, errs, err)
			}
			if !bytes.Equal(got, msg) || fixed != errs {
				t.Fatalf("RS k=%d nsym=%d errs=%d: fixed %d, wrong message", tc.k, tc.nsym, errs, fixed)
			}
		}
	}
}

func TestRSReportsTooManyErrors(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	msg := []byte("https://dseung001.github.io/")
	const nsym = 142
	code, err := RSEncode(msg, nsym)
	if err != nil {
		t.Fatal(err)
	}
	for range 50 {
		bad := corrupt(code, nsym/2+1+r.IntN(30), r)
		got, _, err := RSDecode(bad, nsym)
		if !errors.Is(err, ErrUncorrectable) {
			t.Fatalf("got error %v", err)
		}
		if !bytes.Equal(got, bad[:len(msg)]) {
			t.Fatal("failed decode must return the received bytes")
		}
	}
}

func TestRSEncodeRejectsLongCodeword(t *testing.T) {
	if _, err := RSEncode(make([]byte, 224), 32); err == nil {
		t.Fatal("expected length error")
	}
}

func TestRSTraceFindsInjectedErrors(t *testing.T) {
	msg := []byte("https://dseung001.github.io/")
	code, err := RSEncode(msg, 32)
	if err != nil {
		t.Fatal(err)
	}
	bad := bytes.Clone(code)
	wantPos := []int{0, 7, 27, 28, 59}
	wantMag := []byte{0x01, 0x80, 0xff, 0x3c, 0x5a}
	for i, p := range wantPos {
		bad[p] ^= wantMag[i]
	}

	tr, err := RSDecodeTrace(bad, 32)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Clean || len(tr.Locator) != len(wantPos)+1 {
		t.Fatalf("clean %v, locator %x", tr.Clean, tr.Locator)
	}
	if !slices.Equal(tr.Positions, wantPos) || !bytes.Equal(tr.Magnitudes, wantMag) {
		t.Fatalf("positions %v, magnitudes %x", tr.Positions, tr.Magnitudes)
	}
	if !bytes.Equal(tr.Received, bad) || !bytes.Equal(tr.Corrected, code) || !bytes.Equal(tr.Message, msg) {
		t.Fatalf("received %x, corrected %x, message %q", tr.Received, tr.Corrected, tr.Message)
	}
	if slices.ContainsFunc(tr.Recheck, func(b byte) bool { return b != 0 }) {
		t.Fatalf("recheck %x", tr.Recheck)
	}
}

func TestRSTraceClean(t *testing.T) {
	msg := []byte("watermark")
	code, _ := RSEncode(msg, 32)
	tr, err := RSDecodeTrace(code, 32)
	if err != nil || !tr.Clean || tr.Positions != nil || !bytes.Equal(tr.Message, msg) {
		t.Fatalf("err %v, trace %+v", err, tr)
	}
}

func TestRSTraceReportsFailure(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	msg := []byte("https://dseung001.github.io/")
	code, _ := RSEncode(msg, 32)
	for range 200 {
		bad := corrupt(code, 17+r.IntN(20), r)
		tr, err := RSDecodeTrace(bad, 32)
		if err == nil {
			continue // 다른 코드워드로 오정정된 경우
		}
		if !errors.Is(err, ErrUncorrectable) || tr.FailedAt == "" {
			t.Fatalf("err %v, failedAt %q", err, tr.FailedAt)
		}
		if !bytes.Equal(tr.Message, bad[:len(msg)]) || tr.Corrected != nil {
			t.Fatalf("message %x, corrected %x", tr.Message, tr.Corrected)
		}
	}
}
