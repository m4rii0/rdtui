package download

import (
	"errors"
	"reflect"
	"testing"
)

func TestRenderCommandSubstitutesTemplateValues(t *testing.T) {
	got, err := RenderCommand([]string{"aria2c", "--dir", "{{dir}}", "{{url}}", "{{filename}}"}, TemplateData{URL: "https://example.com/file", Dir: "/tmp", Filename: "file.mkv"})
	if err != nil {
		t.Fatalf("RenderCommand() error = %v", err)
	}
	want := []string{"aria2c", "--dir", "/tmp", "https://example.com/file", "file.mkv"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RenderCommand() = %v, want %v", got, want)
	}
}

func TestRenderCommandRequiresTemplate(t *testing.T) {
	_, err := RenderCommand(nil, TemplateData{})
	if !errors.Is(err, ErrNoCommandConfigured) {
		t.Fatalf("RenderCommand() error = %v, want %v", err, ErrNoCommandConfigured)
	}
}
