package pixels

import "testing"

// The cases are pydicom.tests.pixels.test_processing.TestExpandSegmentedLUT.

func TestExpandSegmentedLUTDiscrete(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		data []int
		want []int
	}{
		{"one", []int{0, 1, 0}, []int{0}},
		{"two", []int{0, 2, 0, 112}, []int{0, 112}},
		{"negative", []int{0, 2, 0, -112}, []int{0, -112}},
		{"trailing null", []int{0, 2, 0, 112, 0, 0}, []int{0, 112}},
		{"trailing null negative", []int{0, 2, 0, -112, 0, 0}, []int{0, -112}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := ExpandSegmentedLUT(c.data, 16, true, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			if !intsEqual(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestExpandSegmentedLUTLinear(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		data []int
		want []int
	}{
		{"positive slope", []int{0, 2, 0, 28672, 1, 5, 49152}, []int{0, 28672, 32768, 36864, 40960, 45056, 49152}},
		{"positive slope from negative", []int{0, 1, -400, 1, 5, 0}, []int{-400, -320, -240, -160, -80, 0}},
		{"flat", []int{0, 2, 0, 28672, 1, 5, 28672}, []int{0, 28672, 28672, 28672, 28672, 28672, 28672}},
		{"flat negative", []int{0, 1, -100, 1, 5, -100}, []int{-100, -100, -100, -100, -100, -100}},
		{"negative slope", []int{0, 2, 0, 49152, 1, 5, 28672}, []int{0, 49152, 45056, 40960, 36864, 32768, 28672}},
		{"negative slope to -400", []int{0, 1, 0, 1, 5, -400}, []int{0, -80, -160, -240, -320, -400}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := ExpandSegmentedLUT(c.data, 16, true, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			if !intsEqual(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}

	// Positive slope with a fractional step. pydicom only checks the length.
	got, err := ExpandSegmentedLUT([]int{0, 1, 163, 1, 48, 255}, 16, true, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1+48 {
		t.Fatalf("len = %d, want %d", len(got), 49)
	}
}

func TestExpandSegmentedLUTIndirect8(t *testing.T) {
	t.Parallel()
	ref := []int{0, 112, 128, 144, 160, 176, 192, 192, 192, 192, 192, 192}

	got, err := ExpandSegmentedLUT([]int{0, 2, 0, 112, 1, 5, 192, 2, 1, 4, 0, 0, 0}, 8, true, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !intsEqual(got, ref) {
		t.Fatalf("little endian got %v, want %v", got, ref)
	}

	got, err = ExpandSegmentedLUT([]int{0, 2, 0, 112, 2, 1, 0, 0, 0, 0}, 8, true, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !intsEqual(got, []int{0, 112, 0, 112}) {
		t.Fatalf("little endian repeat got %v", got)
	}

	got, err = ExpandSegmentedLUT([]int{0, 2, 0, 112, 1, 5, 192, 2, 1, 0, 4, 0, 0}, 8, false, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !intsEqual(got, ref) {
		t.Fatalf("big endian got %v, want %v", got, ref)
	}

	got, err = ExpandSegmentedLUT([]int{0, 2, 0, 112, 2, 1, 0, 0, 0, 0}, 8, false, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !intsEqual(got, []int{0, 112, 0, 112}) {
		t.Fatalf("big endian repeat got %v", got)
	}
}

func TestExpandSegmentedLUTIndirect8LongOffset(t *testing.T) {
	t.Parallel()
	// 0x0100 0x0302 is byte offset 66051 in little endian.
	data := make([]int, 0, 22017*3+6)
	for i := 0; i < 22017; i++ {
		data = append(data, 0, 1, 0)
	}
	data = append(data, 0, 2, 1, 2, 2, 1, 3, 2, 1, 0)
	got, err := ExpandSegmentedLUT(data, 8, true, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := append(repeat(0, 22017), 1, 2, 1, 2)
	if !intsEqual(got, want) {
		t.Fatalf("len %d, tail %v", len(got), tail(got, 8))
	}

	data = make([]int, 0, 22017*3+6)
	for i := 0; i < 22017; i++ {
		data = append(data, 0, 1, 0)
	}
	data = append(data, 0, 2, 1, 2, 2, 1, 2, 3, 0, 1)
	got, err = ExpandSegmentedLUT(data, 8, false, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !intsEqual(got, want) {
		t.Fatalf("big endian len %d, tail %v", len(got), tail(got, 8))
	}
}

func TestExpandSegmentedLUTIndirect16(t *testing.T) {
	t.Parallel()
	got, err := ExpandSegmentedLUT([]int{0, 2, 0, 112, 1, 5, 192, 2, 2, 0, 0}, 16, true, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	one := []int{0, 112, 128, 144, 160, 176, 192}
	want := append(append([]int{}, one...), one...)
	if !intsEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	got, err = ExpandSegmentedLUT([]int{0, 2, 0, 112, 1, 5, 192, 2, 1, 4, 0}, 16, true, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	want = []int{0, 112, 128, 144, 160, 176, 192, 192, 192, 192, 192, 192}
	if !intsEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestExpandSegmentedLUTRejectsBadFirstSegment(t *testing.T) {
	t.Parallel()
	_, err := ExpandSegmentedLUT([]int{1, 5, 49152}, 16, true, 0, 0)
	if err == nil || !contains(err.Error(), "the first segment cannot be a linear segment") {
		t.Fatalf("linear first: %v", err)
	}
	_, err = ExpandSegmentedLUT([]int{2, 5, 2, 0}, 16, true, 0, 0)
	if err == nil || !contains(err.Error(), "the first segment cannot be an indirect segment") {
		t.Fatalf("indirect first: %v", err)
	}
	_, err = ExpandSegmentedLUT([]int{3, 5, 49152}, 16, true, 0, 0)
	if err == nil || !contains(err.Error(), "unknown segment type '3'") {
		t.Fatalf("unknown opcode: %v", err)
	}
}

func intsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func repeat(v, n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = v
	}
	return out
}

func tail(v []int, n int) []int {
	if len(v) < n {
		return v
	}
	return v[len(v)-n:]
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
