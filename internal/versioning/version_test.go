package versioning

import "testing"

func TestParseAndCompare(t *testing.T) {
	parsed, err := Parse("1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.String() != "1.2.3" {
		t.Fatalf("version = %q", parsed.String())
	}
	if Compare(parsed, MustParse("1.2.4")) >= 0 || Compare(parsed, MustParse("1.2.2")) <= 0 {
		t.Fatal("version comparison is incorrect")
	}
}

func TestParseRejectsNonReleaseVersions(t *testing.T) {
	for _, value := range []string{"v1.2.3", "1.2", "1.2.3-beta.1", "1.2.3+build", "01.2.3", "1.-2.3"} {
		if _, err := Parse(value); err == nil {
			t.Fatalf("accepted invalid version %q", value)
		}
	}
}

func TestRangeContains(t *testing.T) {
	r := Range{Min: MustParse("1.0.0"), MaxExclusive: MustParse("1.1.0")}
	if !r.Contains(MustParse("1.0.9")) || r.Contains(MustParse("1.1.0")) || r.Contains(MustParse("0.9.9")) {
		t.Fatal("unexpected range result")
	}
}
