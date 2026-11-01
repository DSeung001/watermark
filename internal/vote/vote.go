// Package vote는 같은 비트를 여러 자리에 반복해 넣었을 때 쓰는 다수결 집계다.
package vote

// Tally는 메시지 비트별로 읽은 복사본 수(Copies)와 그중 1의 수(Ones)를 담는다.
type Tally struct {
	Ones   []int `json:"ones"`
	Copies []int `json:"copies"`
}

// New는 nBits개 비트를 위한 빈 집계를 만든다.
func New(nBits int) Tally {
	return Tally{Ones: make([]int, nBits), Copies: make([]int, nBits)}
}

// Add는 i번째 메시지 비트의 복사본 하나를 기록한다.
func (t Tally) Add(i int, bit uint8) {
	t.Copies[i]++
	t.Ones[i] += int(bit)
}

// Bit은 i번째 비트의 다수결 값이다. 동률이면 0으로 읽는다.
func (t Tally) Bit(i int) uint8 {
	if t.Ones[i]*2 > t.Copies[i] {
		return 1
	}
	return 0
}

// Bytes는 다수결로 복원한 바이트열이다. 각 바이트는 MSB부터 채운다.
func (t Tally) Bytes() []byte {
	out := make([]byte, len(t.Ones)/8)
	for i := range len(out) * 8 {
		out[i/8] = out[i/8]<<1 | t.Bit(i)
	}
	return out
}

// Errors는 다수결 없이 복사본 하나하나를 want와 비교했을 때의 오류 수와 전체 수다.
func (t Tally) Errors(want []byte) (errs, total int) {
	for i := range t.Ones {
		total += t.Copies[i]
		if Bit(want, i) == 1 {
			errs += t.Copies[i] - t.Ones[i]
		} else {
			errs += t.Ones[i]
		}
	}
	return errs, total
}

// Bit은 data의 i번째 비트다. 각 바이트의 MSB가 먼저다.
func Bit(data []byte, i int) uint8 {
	return (data[i/8] >> (7 - i%8)) & 1
}
