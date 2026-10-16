package demo

import (
	"testing"

	"github.com/m4rii0/rdtui/pkg/models"
)

func TestAddedTorrentFilenames(t *testing.T) {
	for _, tt := range []struct{ method, input, want string }{
		{"magnet", "magnet:?xt=hash", "New Magnet Torrent"},
		{"magnet", "magnet:?dn=My+Movie&tr=tracker", "My Movie"},
		{"magnet", "magnet:?dn=My+Movie", "My Movie"},
		{"magnet", "magnet:?dn=&tr=tracker", ""},
		{"url", "https://example.com/file.torrent", "file.torrent"},
		{"url", "https://example.com/dir/", "https://example.com/dir/"},
		{"url", "file.torrent", "file.torrent"},
		{"file", "/dir/file.torrent", "file"},
		{"file", "file.torrent", "file"},
		{"file", "/dir/", ""},
	} {
		t.Run(tt.method+"/"+tt.input, func(t *testing.T) {
			s := &Service{}
			var added models.AddTorrentResult
			var err error
			switch tt.method {
			case "magnet":
				added, err = s.AddMagnet(t.Context(), tt.input)
			case "url":
				added, err = s.AddTorrentURL(t.Context(), tt.input)
			case "file":
				results := s.ImportTorrentFiles(t.Context(), []string{tt.input})
				added.ID, err = results[0].TorrentID, results[0].Err
			}
			if err != nil {
				t.Fatal(err)
			}
			info, err := s.TorrentInfo(t.Context(), added.ID)
			if err != nil {
				t.Fatal(err)
			}
			if info.Filename != tt.want {
				t.Errorf("filename = %q, want %q", info.Filename, tt.want)
			}
		})
	}
}
