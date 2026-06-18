package internal

import "testing"

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in              string
		wantOK          bool
		maj, min, patch int
	}{
		{"1.0.0", true, 1, 0, 0},
		{"v2.4.0", true, 2, 4, 0},
		{"2.4.0", true, 2, 4, 0},
		{"v2.4.0-3-gabc1234", true, 2, 4, 0}, // git-describe suffix dropped
		{"2.4.0-dirty", true, 2, 4, 0},
		{"10.20.30", true, 10, 20, 30},
		{"dev", false, 0, 0, 0},
		{"", false, 0, 0, 0},
		{"garbage", false, 0, 0, 0},
		{"1.2", false, 0, 0, 0}, // need all three components
	}
	for _, c := range cases {
		v, ok := parseVersion(c.in)
		if ok != c.wantOK {
			t.Errorf("parseVersion(%q) ok = %v, want %v", c.in, ok, c.wantOK)
			continue
		}
		if ok && (v.major != c.maj || v.minor != c.min || v.patch != c.patch) {
			t.Errorf("parseVersion(%q) = %d.%d.%d, want %d.%d.%d",
				c.in, v.major, v.minor, v.patch, c.maj, c.min, c.patch)
		}
	}
}

func TestShouldTakeOver(t *testing.T) {
	cases := []struct {
		local, remote string
		want          bool
	}{
		// Local strictly newer than a parseable remote → take over.
		{"2.0.0", "1.0.0", true},
		{"2.4.1", "2.4.0", true},
		{"v2.4.0", "v2.3.9", true},
		{"1.1.0", "1.0.9", true},
		// Equal / older → follow.
		{"2.0.0", "2.0.0", false},
		{"1.0.0", "2.0.0", false},
		{"2.4.0", "2.4.1", false},
		// Unparseable on either side → follow (no fight, no flapping).
		{"dev", "2.0.0", false},
		{"2.0.0", "dev", false},
		{"dev", "dev", false},
		{"", "1.0.0", false},
		{"1.0.0", "", false},
		// git-describe suffix compares on the base version (equal base → follow).
		{"2.4.0-3-gabc", "2.4.0", false},
	}
	for _, c := range cases {
		got := shouldTakeOver(c.local, c.remote)
		if got != c.want {
			t.Errorf("shouldTakeOver(%q, %q) = %v, want %v", c.local, c.remote, got, c.want)
		}
	}
}
