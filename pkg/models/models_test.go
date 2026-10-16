package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTorrentInfoJSONOptionalFields(t *testing.T) {
	ended := time.Date(2026, time.October, 16, 12, 30, 0, 0, time.UTC)
	for _, tt := range []struct {
		name string
		info TorrentInfo
		want map[string]string
	}{
		{name: "zero values omitted"},
		{
			name: "nonzero values retained",
			info: TorrentInfo{Ended: ended, Seeders: 3, Speed: 1024},
			want: map[string]string{"ended": `"2026-10-16T12:30:00Z"`, "seeders": "3", "speed": "1024"},
		},
		{
			name: "negative numbers retained",
			info: TorrentInfo{Seeders: -1, Speed: -1},
			want: map[string]string{"seeders": "-1", "speed": "-1"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Exercise the existing encoding/json contract, not json/v2 defaults.
			data, err := json.Marshal(tt.info)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(data, &fields); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			for _, key := range []string{"ended", "seeders", "speed"} {
				got, present := fields[key]
				want, wantPresent := tt.want[key]
				if present != wantPresent || string(got) != want {
					t.Errorf("field %q = %s (present %t), want %s (present %t)", key, got, present, want, wantPresent)
				}
			}
			for key, want := range map[string]string{"bytes": "0", "links": "null", "files": "null"} {
				if got := string(fields[key]); got != want {
					t.Errorf("field %q = %s, want %s", key, got, want)
				}
			}
			var decoded TorrentInfo
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("round-trip Unmarshal() error = %v", err)
			}
			if !decoded.Ended.Equal(tt.info.Ended) || decoded.Seeders != tt.info.Seeders || decoded.Speed != tt.info.Speed {
				t.Errorf("round-trip optional fields = %+v, want %+v", decoded, tt.info)
			}
		})
	}
}
