package main

import (
	"testing"
	"time"
)

func TestChannelExportFilenameSanitizesNameAndIncludesUTCTimestamp(t *testing.T) {
	exportedAt := time.Date(2026, time.September, 17, 2, 3, 4, 0, time.FixedZone("west", -7*60*60))
	tests := []struct {
		name string
		want string
	}{
		{name: `../../Team \ "Quarterly"\r\nreport`, want: "channel-Team-Quarterly-r-nreport-2026-09-17-090304.html"},
		{name: `<script>`, want: "channel-script-2026-09-17-090304.html"},
		{name: `...---___`, want: "channel-channel-2026-09-17-090304.html"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := channelExportFilename(tt.name, exportedAt); got != tt.want {
				t.Errorf("channelExportFilename() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDMExportFilenameRemainsUnchanged(t *testing.T) {
	exportedAt := time.Date(2026, time.September, 16, 21, 30, 45, 0, time.UTC)
	if got, want := exportFilename("../master/path", `target\\name`, exportedAt), "dm-master-path-target-name-2026-09-16-213045.html"; got != want {
		t.Errorf("exportFilename() = %q, want %q", got, want)
	}
}
