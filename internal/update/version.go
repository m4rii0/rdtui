package update

import (
	"fmt"
	"strings"

	"golang.org/x/mod/semver"
)

func NormalizeReleasedVersion(value string) (string, error) {
	version := strings.TrimSpace(value)
	if version == "" || version == "dev" {
		return "", fmt.Errorf("version %q cannot be checked for self-update; only released semver versions can be checked", value)
	}
	if !semver.IsValid(version) {
		return "", fmt.Errorf("version %q is not valid semver", value)
	}
	if semver.Prerelease(version) != "" || semver.Build(version) != "" {
		return "", fmt.Errorf("version %q is not a stable released version", value)
	}
	canonical := semver.Canonical(version)
	if canonical == "" {
		return "", fmt.Errorf("version %q is not valid semver", value)
	}
	return canonical, nil
}

func IsNewer(current, latest string) bool {
	return semver.Compare(current, latest) < 0
}
