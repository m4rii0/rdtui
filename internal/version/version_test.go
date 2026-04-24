package version

import "testing"

func TestCurrentPrefersEmbeddedReleaseVersion(t *testing.T) {
	got := current("v1.2.3", "v1.2.4")
	if got != "v1.2.3" {
		t.Fatalf("current = %q, want v1.2.3", got)
	}
}

func TestCurrentFallsBackToModuleVersion(t *testing.T) {
	got := current("dev", "v1.2.3")
	if got != "v1.2.3" {
		t.Fatalf("current = %q, want v1.2.3", got)
	}
}

func TestCurrentKeepsDevForLocalBuilds(t *testing.T) {
	tests := []struct {
		embedded string
		module   string
	}{
		{"dev", ""},
		{"dev", "(devel)"},
		{"", "(devel)"},
	}

	for _, tt := range tests {
		got := current(tt.embedded, tt.module)
		if got != "dev" {
			t.Fatalf("current(%q, %q) = %q, want dev", tt.embedded, tt.module, got)
		}
	}
}
