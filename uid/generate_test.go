package uid

import (
	"strings"
	"testing"
)

func TestGenerateUID_DefaultPrefix(t *testing.T) {
	t.Parallel()
	uid, err := GenerateUID()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(uid), RootUID) {
		t.Fatalf("prefix = %q, want %q…", uid, RootUID)
	}
	if len(uid) > 64 {
		t.Fatalf("len = %d, want ≤ 64", len(uid))
	}
	if err := Validate(string(uid)); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestGenerateUID_UUIDPrefix(t *testing.T) {
	t.Parallel()
	uid, err := GenerateUID(WithUUIDPrefix())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(uid), "2.25.") {
		t.Fatalf("prefix = %q, want 2.25.…", uid)
	}
	if len(uid) > 64 {
		t.Fatalf("len = %d, want ≤ 64", len(uid))
	}
	if err := Validate(string(uid)); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestGenerateUID_InvalidPrefix(t *testing.T) {
	t.Parallel()
	invalid := []string{
		strings.Repeat("1", 54) + ".",
		"",
		".",
		"1",
		"1.2",
		"1.2..3.",
		"1.a.2.",
		"1.01.1.",
	}
	for _, prefix := range invalid {
		t.Run(prefix, func(t *testing.T) {
			t.Parallel()
			_, err := GenerateUID(WithPrefix(prefix))
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestGenerateUID_ValidPrefix(t *testing.T) {
	t.Parallel()
	valid := []string{
		"0.",
		"1.",
		"1.23.",
		"1.0.23.",
		strings.Repeat("1", 53) + ".",
		"1.2.3.444444.",
	}
	for _, prefix := range valid {
		t.Run(prefix, func(t *testing.T) {
			t.Parallel()
			uid, err := GenerateUID(WithPrefix(prefix))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(string(uid), prefix) {
				t.Fatalf("uid = %q, want prefix %q", uid, prefix)
			}
			if len(uid) > 64 {
				t.Fatalf("len = %d, want ≤ 64", len(uid))
			}
		})
	}
}

func TestGenerateUID_RandomDiffers(t *testing.T) {
	t.Parallel()
	a, err := GenerateUID()
	if err != nil {
		t.Fatal(err)
	}
	b, err := GenerateUID()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatalf("expected different UIDs, both %q", a)
	}
}

func TestGenerateUID_EntropyDeterministic(t *testing.T) {
	t.Parallel()
	const want = "1.2.826.0.1.3680043.8.498.87507166259346337659265156363895084463"
	got, err := GenerateUID(WithEntropy("lorem", "ipsum"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if len(got) != 64 {
		t.Fatalf("len = %d, want 64", len(got))
	}
	again, err := GenerateUID(WithEntropy("lorem", "ipsum"))
	if err != nil {
		t.Fatal(err)
	}
	if again != got {
		t.Fatalf("not deterministic: %q vs %q", again, got)
	}
}

func TestGenerateUID_UUIDManyValid(t *testing.T) {
	t.Parallel()
	for i := 0; i < 1000; i++ {
		uid, err := GenerateUID(WithUUIDPrefix())
		if err != nil {
			t.Fatal(err)
		}
		if err := Validate(string(uid)); err != nil {
			t.Fatalf("iteration %d: %v (%q)", i, err, uid)
		}
	}
}

func TestMustGenerateUID(t *testing.T) {
	t.Parallel()
	uid := MustGenerateUID(WithEntropy("a", "b"))
	if err := Validate(string(uid)); err != nil {
		t.Fatal(err)
	}
}
