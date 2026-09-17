package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var unsafeFilenameCharacters = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizedFilenamePart(value, fallback string) string {
	value = strings.Trim(unsafeFilenameCharacters.ReplaceAllString(value, "-"), ".-_")
	if value == "" {
		return fallback
	}
	return value
}

func exportFilename(requester, target string, exportedAt time.Time) string {
	return fmt.Sprintf("dm-%s-%s-%s.html",
		sanitizedFilenamePart(requester, "user"),
		sanitizedFilenamePart(target, "user"),
		exportedAt.UTC().Format("2006-01-02-150405"))
}

func channelExportFilename(name string, exportedAt time.Time) string {
	return fmt.Sprintf("channel-%s-%s.html",
		sanitizedFilenamePart(name, "channel"),
		exportedAt.UTC().Format("2006-01-02-150405"))
}
