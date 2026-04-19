package app

import (
	"reflect"
	"testing"

	"github.com/m4rii0/rdtui/pkg/models"
)

func TestDefaultFileSelectionChoosesLargestFile(t *testing.T) {
	info := models.TorrentInfo{Files: []models.TorrentFile{{ID: 1, Bytes: 100}, {ID: 2, Bytes: 200}, {ID: 3, Bytes: 150}}}
	got := DefaultFileSelection(info)
	want := []int{2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DefaultFileSelection() = %v, want %v", got, want)
	}
}

func TestValidTorrentFilesFiltersAndDeduplicates(t *testing.T) {
	valid, invalid := ValidTorrentFiles([]string{"/tmp/a.torrent", "/tmp/a.torrent", "/tmp/b.txt", "/tmp/c.TORRENT"})
	if !reflect.DeepEqual(valid, []string{"/tmp/a.torrent", "/tmp/c.TORRENT"}) {
		t.Fatalf("valid = %v", valid)
	}
	if !reflect.DeepEqual(invalid, []string{"/tmp/b.txt"}) {
		t.Fatalf("invalid = %v", invalid)
	}
}
