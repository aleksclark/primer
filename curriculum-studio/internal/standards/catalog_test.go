package standards

import "testing"

func TestImportCodeAndSource(t *testing.T) {
	code := ImportCode("tennessee", "Grade 6 Mathematics")
	if code != "tennessee-grade-6-mathematics" {
		t.Fatalf("unexpected import code %q", code)
	}
	if SourceFromCode(code) != "tennessee" {
		t.Fatalf("source did not round-trip")
	}
	if ImportCode("custom", "###") != "custom-catalog" {
		t.Fatalf("empty title slug should use catalog fallback")
	}
}

func TestValidSource(t *testing.T) {
	for _, source := range []string{"tennessee", "common_core", "custom"} {
		if !ValidSource(source) {
			t.Errorf("expected %q to be valid", source)
		}
	}
	if ValidSource("lms") {
		t.Fatal("unexpected open source value")
	}
}
