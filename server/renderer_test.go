package main

import (
	"strings"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

func TestRenderHTMLExportIncludesRequiredFields(t *testing.T) {
	exportedAt := time.Date(2026, time.September, 16, 12, 34, 56, 0, time.FixedZone("test", 2*60*60))
	posts := []*model.Post{
		{Id: "root-id", UserId: "requester-id", CreateAt: 1_700_000_000_000, Message: "root message"},
		{Id: "reply-id", RootId: "root-id", UserId: "target-id", CreateAt: 1_700_000_001_234, Message: "reply message"},
	}
	attachments := map[string][]AttachmentMetadata{
		"reply-id": {{ID: "file-id", Filename: "notes.txt", Size: 42, MIMEType: "text/plain"}},
	}

	got, err := renderHTMLExport(
		&model.User{Id: "requester-id", Username: "requester"},
		&model.User{Id: "target-id", Username: "other"},
		exportedAt,
		posts,
		attachments,
	)
	if err != nil {
		t.Fatalf("renderHTMLExport returned an error: %v", err)
	}

	html := string(got)
	for _, required := range []string{
		`<!doctype html>`, `<meta charset="utf-8">`, "@requester", "@other",
		"2026-09-16T10:34:56Z", "At most the latest 100 non-deleted messages are included.",
		"2023-11-14T22:13:20Z", "root message", "Root post",
		"2023-11-14T22:13:21.234Z", "reply message", "Reply to post", "root-id",
		"notes.txt", "42 bytes", "text/plain", "file-id",
	} {
		if !strings.Contains(html, required) {
			t.Errorf("rendered HTML does not contain %q\n%s", required, html)
		}
	}
}

func TestRenderHTMLExportEscapesAllUserControlledFields(t *testing.T) {
	attack := `<script>alert("export")</script>`
	got, err := renderHTMLExport(
		&model.User{Id: "requester-id", Username: attack},
		&model.User{Id: "target-id", Username: "target"},
		time.Unix(0, 0),
		[]*model.Post{{Id: attack, RootId: attack, UserId: "requester-id", Message: attack}},
		map[string][]AttachmentMetadata{attack: {{ID: attack, Filename: attack, MIMEType: attack}}},
	)
	if err != nil {
		t.Fatalf("renderHTMLExport returned an error: %v", err)
	}

	html := string(got)
	if strings.Contains(html, attack) {
		t.Fatalf("rendered HTML contains unescaped input: %s", html)
	}
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Fatalf("rendered HTML does not contain escaped input: %s", html)
	}
	if strings.Contains(html, "<script>") {
		t.Fatalf("rendered HTML contains a script element: %s", html)
	}
}
