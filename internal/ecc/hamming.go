// Package ecc는 워터마크 메시지에 쓰는 오류 정정 부호다.
package ecc

import "watermark/internal/vote"

const (
	hammingN    = 7 // 코드워드 비트 수
	hammingData = 4 // 데이터 비트 수
)

// 코드워드 위치(1부터)는 P1 P2 D1 P4 D2 D3 D4 순서다.
// 패리티는 2의 거듭제곱 자리에 놓이고, 나머지가 데이터 자리다.
var hammingDataPos = [hammingData]int{3, 5, 6, 7}

// Hamming74Len은 n바이트를 부호화한 결과의 바이트 수다. 남는 비트는 0으로 채운다.
func Hamming74Len(n int) int {
	return (n*8/hammingData*hammingN + 7) / 8
}

// Encode74는 data를 4비트씩 Hamming(7,4) 짝수 패리티로 부호화한다.
func Encode74(data []byte) []byte {
	out := make([]byte, Hamming74Len(len(data)))
	groups := len(data) * 8 / hammingData
	for g := range groups {
		var word [hammingN + 1]uint8
		syndrome := 0
		for i, pos := range hammingDataPos {
			word[pos] = vote.Bit(data, g*hammingData+i)
			if word[pos] == 1 {
				syndrome ^= pos
			}
		}
		// 값이 1인 자리 번호의 XOR이 0이 되도록 패리티를 고른다.
		word[1] = uint8(syndrome & 1)
		word[2] = uint8(syndrome >> 1 & 1)
		word[4] = uint8(syndrome >> 2 & 1)
		for pos := 1; pos <= hammingN; pos++ {
			setBit(out, g*hammingN+pos-1, word[pos])
		}
	}
	return out
}

// Decode74는 n바이트 데이터를 복호한다. fixed는 비트를 고친 코드워드 수다.
// 코드워드 하나에 2비트 이상 틀리면 다른 자리를 잘못 고칠 수 있다.
func Decode74(coded []byte, n int) (data []byte, fixed int) {
	data = make([]byte, n)
	groups := n * 8 / hammingData
	for g := range groups {
		var word [hammingN + 1]uint8
		syndrome := 0
		for pos := 1; pos <= hammingN; pos++ {
			word[pos] = vote.Bit(coded, g*hammingN+pos-1)
			if word[pos] == 1 {
				syndrome ^= pos
			}
		}
		// 신드롬이 틀린 자리의 번호다.
		if syndrome != 0 {
			word[syndrome] ^= 1
			fixed++
		}
		for i, pos := range hammingDataPos {
			setBit(data, g*hammingData+i, word[pos])
		}
	}
	return data, fixed
}

func setBit(data []byte, i int, bit uint8) {
	data[i/8] |= bit << (7 - i%8)
}
