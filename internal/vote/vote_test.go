package vote

import "testing"

func TestErrorsComparesOnlyWantBits(t *testing.T) {
	tally := New(16)
	for i, bit := range []uint8{1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0} {
		tally.Add(i, bit)
	}
	if errs, total := tally.Errors([]byte{0xff, 0xff}); errs != 8 || total != 16 {
		t.Fatalf("all bits: errs %d, total %d", errs, total)
	}
	if errs, total := tally.Errors([]byte{0xff}); errs != 0 || total != 8 {
		t.Fatalf("prefix: errs %d, total %d", errs, total)
	}
}
