package update

import (
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
)

var runningVersion atomic.Value

func setCurrentVersion(raw string) {
	runningVersion.Store(strings.TrimSpace(raw))
}

// Semver is a parsed major.minor.patch version.
type Semver struct {
	Major int
	Minor int
	Patch int
}

var semverTagRe = regexp.MustCompile(`(?i)^v?(\d+)\.(\d+)\.(\d+)(?:-[0-9a-z._-]+)?$`)

// ParseReleaseTag extracts the normalized "X.Y.Z" version, the trimmed tag and the numeric Semver.
func ParseReleaseTag(raw string) (version string, tag string, sv Semver, ok bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", Semver{}, false
	}
	m := semverTagRe.FindStringSubmatch(trimmed)
	if len(m) != 4 {
		return "", "", Semver{}, false
	}
	maj, err1 := strconv.Atoi(m[1])
	minor, err2 := strconv.Atoi(m[2])
	patch, err3 := strconv.Atoi(m[3])
	if err1 != nil || err2 != nil || err3 != nil {
		return "", "", Semver{}, false
	}
	return m[1] + "." + m[2] + "." + m[3], trimmed, Semver{Major: maj, Minor: minor, Patch: patch}, true
}

// CurrentVersion returns the normalized running application version.
func CurrentVersion() string {
	if raw, ok := runningVersion.Load().(string); ok {
		if ver, _, _, valid := ParseReleaseTag(raw); valid {
			return ver
		}
	}
	return "0.0.0"
}

// CompareVersion returns -1 if a < b, 0 if a == b and +1 if a > b.
func CompareVersion(a, b string) int {
	_, _, left, okA := ParseReleaseTag(a)
	_, _, right, okB := ParseReleaseTag(b)
	if !okA && !okB {
		return 0
	}
	if !okA {
		return -1
	}
	if !okB {
		return 1
	}
	for _, pair := range [][2]int{{left.Major, right.Major}, {left.Minor, right.Minor}, {left.Patch, right.Patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return 0
}
