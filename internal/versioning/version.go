package versioning

import (
	"fmt"
	"strconv"
	"strings"
)

// Version is the release-only MAJOR.MINOR.PATCH subset of Semantic Versioning.
// semdiff does not currently publish prerelease or build metadata versions.
type Version struct {
	Major int
	Minor int
	Patch int
}

func Parse(value string) (Version, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("must use MAJOR.MINOR.PATCH")
	}
	values := [3]int{}
	for index, part := range parts {
		if part == "" || len(part) > 1 && part[0] == '0' {
			return Version{}, fmt.Errorf("must use canonical non-negative integers")
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return Version{}, fmt.Errorf("must use MAJOR.MINOR.PATCH without prerelease or build metadata")
			}
		}
		parsed, err := strconv.Atoi(part)
		if err != nil {
			return Version{}, fmt.Errorf("parse component %q: %w", part, err)
		}
		values[index] = parsed
	}
	return Version{Major: values[0], Minor: values[1], Patch: values[2]}, nil
}

func MustParse(value string) Version {
	parsed, err := Parse(value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func Compare(left, right Version) int {
	leftParts := [...]int{left.Major, left.Minor, left.Patch}
	rightParts := [...]int{right.Major, right.Minor, right.Patch}
	for index := range leftParts {
		if leftParts[index] < rightParts[index] {
			return -1
		}
		if leftParts[index] > rightParts[index] {
			return 1
		}
	}
	return 0
}

type Range struct {
	Min          Version
	MaxExclusive Version
}

func (r Range) Contains(version Version) bool {
	return Compare(version, r.Min) >= 0 && Compare(version, r.MaxExclusive) < 0
}
