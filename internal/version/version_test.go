package version

import "testing"

func TestCompare(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{"given equal semantic versions then they compare equal", "1.2.3", "1.2.3", 0},
		{"given a higher minor then it is greater", "1.10.0", "1.9.9", 1},
		{"given a lower patch then it is smaller", "1.0.0", "1.0.1", -1},
		{"given a leading v then it is ignored", "v2.0.0", "2.0.0", 0},
		{"given missing parts then they count as zero", "1.2", "1.2.0", 0},
		{"given a pre-release then it sorts before the release", "1.0.0-rc1", "1.0.0", -1},
		{"given non-semantic strings then natural order applies", "GE-Proton11-7", "GE-Proton11-10", -1},
		{"given a semantic and non-semantic version then semantic sorts first", "1.0.0", "latest", -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// When
			got := Compare(tc.a, tc.b)

			// Then
			if got != tc.want {
				t.Fatalf("Compare(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestParse(t *testing.T) {
	t.Run("given a full version when parsed then each part is read", func(t *testing.T) {
		// When
		v, err := Parse("3.14.15-beta")

		// Then
		if err != nil || v.Major != 3 || v.Minor != 14 || v.Patch != 15 || v.PreRelease != "beta" {
			t.Fatalf("Parse = %+v, %v", v, err)
		}
	})

	invalid := []string{"", "banana", "1.x.0", "1.2.3.4", "-1.0.0"}
	for _, in := range invalid {
		t.Run("given "+in+" when parsed then it is rejected", func(t *testing.T) {
			// When
			_, err := Parse(in)

			// Then
			if err == nil {
				t.Fatalf("Parse(%q) accepted an invalid version", in)
			}
		})
	}
}

func TestIsCompatible(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{"given the same major version then they are compatible", "1.0.0", "1.9.3", true},
		{"given different major versions then they are incompatible", "1.0.0", "2.0.0", false},
		{"given an unparseable version then it is incompatible", "1.0.0", "nope", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsCompatible(tc.a, tc.b); got != tc.want {
				t.Fatalf("IsCompatible(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestLatest(t *testing.T) {
	t.Run("given several versions when the latest is requested then the highest is returned", func(t *testing.T) {
		if got := Latest([]string{"1.0.0", "1.10.0", "1.2.0"}); got != "1.10.0" {
			t.Fatalf("Latest = %q", got)
		}
	})
	t.Run("given no versions then the latest is empty", func(t *testing.T) {
		if got := Latest(nil); got != "" {
			t.Fatalf("Latest = %q", got)
		}
	})
}
