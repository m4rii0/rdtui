package tui

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestSplitPathInput(t *testing.T) {
	sep := string(os.PathSeparator)
	for _, tt := range []struct {
		raw    string
		dir    string
		prefix string
	}{
		{raw: "", dir: "."},
		{raw: "file", dir: ".", prefix: "file"},
		{raw: sep},
		{raw: sep + "file", prefix: "file"},
		{raw: "dir" + sep, dir: "dir"},
		{raw: "dir" + sep + "sub" + sep + "file", dir: "dir" + sep + "sub", prefix: "file"},
		{raw: "dir" + sep + sep + "file", dir: "dir" + sep, prefix: "file"},
	} {
		t.Run(tt.raw, func(t *testing.T) {
			dir, prefix := splitPathInput(tt.raw)
			if dir != tt.dir || prefix != tt.prefix {
				t.Errorf("splitPathInput(%q) = (%q, %q), want (%q, %q)", tt.raw, dir, prefix, tt.dir, tt.prefix)
			}
		})
	}
}

func TestFilepathBase(t *testing.T) {
	for _, tt := range []struct{ path, want string }{
		{"", ""},
		{"file.torrent", "file.torrent"},
		{"/dir/file.torrent", "file.torrent"},
		{`C:\dir\file.torrent`, "file.torrent"},
		{`C:\dir/sub\file.torrent`, "file.torrent"},
		{"/dir/", ""},
		{`C:\dir\`, ""},
		{"/", ""},
	} {
		t.Run(tt.path, func(t *testing.T) {
			if got := filepathBase(tt.path); got != tt.want {
				t.Errorf("filepathBase(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestBrowserSortsDirectoriesBeforeTorrentFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"z-dir", "A-dir"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"a.torrent", "B.torrent", "ignored.txt", ".hidden.torrent"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	b := newFileBrowser(dir)
	msg := b.readDirCmd()().(browserReadDirMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	want := []string{"..", "A-dir", "z-dir", "a.torrent", "B.torrent"}
	var names []string
	for _, entry := range msg.entries {
		names = append(names, entry.Name)
	}
	if !slices.Equal(names, want) {
		t.Errorf("directory entries = %v, want %v", names, want)
	}
	b.startEditing()
	names = nil
	for _, entry := range b.editCompletions {
		names = append(names, entry.Name)
	}
	if !slices.Equal(names, want[1:]) {
		t.Errorf("completions = %v, want %v", names, want[1:])
	}
}
