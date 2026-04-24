package version

import (
	"runtime/debug"
	"strings"
)

var Version = "dev"

func Current() string {
	return current(Version, buildInfoVersion())
}

func current(embedded, module string) string {
	embedded = strings.TrimSpace(embedded)
	if embedded != "" && embedded != "dev" {
		return embedded
	}

	module = strings.TrimSpace(module)
	if module != "" && module != "(devel)" {
		return module
	}

	if embedded == "" {
		return "dev"
	}
	return embedded
}

func buildInfoVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	return info.Main.Version
}
