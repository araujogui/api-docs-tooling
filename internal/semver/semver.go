// Package semver coerces loose version strings into versions, like the
// `semver` package's `coerce`.
package semver

import (
	"fmt"
	"regexp"
	"strconv"
)

// Version is a major.minor.patch version.
type Version struct {
	Major int
	Minor int
	Patch int
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// MarshalJSON writes the version as its string, as a SemVer object
// stringifies.
func (v Version) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(v.String())), nil
}

var coercePattern = regexp.MustCompile(`(?:^|[^\d])(\d{1,16})(?:\.(\d{1,16}))?(?:\.(\d{1,16}))?(?:$|[^\d])`)

// Coerce finds the first version-like run of numbers in a string, filling
// in missing parts with zeros. It returns nil when there is none.
func Coerce(s string) *Version {
	m := coercePattern.FindStringSubmatch(s)

	if m == nil {
		return nil
	}

	part := func(i int) int {
		if m[i] == "" {
			return 0
		}

		n, _ := strconv.Atoi(m[i])

		return n
	}

	return &Version{Major: part(1), Minor: part(2), Patch: part(3)}
}

// MustCoerce coerces a version, falling back to 0.0.0.
func MustCoerce(s string) Version {
	if v := Coerce(s); v != nil {
		return *v
	}

	return Version{}
}
