package internal

import (
	"strconv"
	"strings"
)

// semver is a parsed major.minor.patch triple. Pre-release / build / git-describe
// suffixes are intentionally ignored — leader election only needs the release
// ordering, and the version source of truth is `git describe --tags`.
type semver struct {
	major, minor, patch int
}

// parseVersion parses a "vX.Y.Z" / "X.Y.Z" string into a semver, dropping any
// leading "v" and any git-describe / pre-release suffix ("-3-gabc", "-dirty").
// Returns ok=false for "dev", empty, or anything without three numeric
// components — callers treat an unparseable version as "don't fight" (follow).
func parseVersion(s string) (semver, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	// Drop the first suffix marker so "2.4.0-3-gabc" / "2.4.0-dirty" → "2.4.0".
	if i := strings.IndexByte(s, '-'); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return semver{}, false
		}
		nums[i] = n
	}
	return semver{major: nums[0], minor: nums[1], patch: nums[2]}, true
}

// compareSemver returns -1, 0, or 1 for a<b, a==b, a>b.
func compareSemver(a, b semver) int {
	for _, d := range []int{a.major - b.major, a.minor - b.minor, a.patch - b.patch} {
		if d < 0 {
			return -1
		}
		if d > 0 {
			return 1
		}
	}
	return 0
}

// shouldTakeOver reports whether a node running `local` should evict a healthy
// leader running `remote` and take over the port. It returns true ONLY when both
// versions parse cleanly AND local is strictly newer — the stale-binary case
// (issue #26). Equal, older, or any unparseable ("dev", custom, empty) version on
// either side returns false: the node follows the existing leader. Gating on a
// strict-newer comparison keeps the decision stable (the newer binary always
// wins; no two binaries can ping-pong takeovers).
func shouldTakeOver(local, remote string) bool {
	lv, lok := parseVersion(local)
	rv, rok := parseVersion(remote)
	if !lok || !rok {
		return false
	}
	return compareSemver(lv, rv) > 0
}
