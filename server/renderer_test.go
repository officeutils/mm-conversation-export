package main

import (
	"strings"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

func renderTestExport(t *testing.T, posts []*model.Post, attachments map[string][]AttachmentMetadata) string {
	t.Helper()
	got, err := renderHTMLExport(
		&model.User{Id: "master-id", Username: "master"},
		&model.User{Id: "tester-id", Username: "tester"},
		time.Date(2026, time.September, 16, 12, 34, 56, 0, time.UTC),
		defaultMaxExportPosts,
		posts,
		attachments,
	)
	if err != nil {
		t.Fatalf("renderHTMLExport returned an error: %v", err)
	}
	return string(got)
}

func assertInOrder(t *testing.T, text string, values ...string) {
	t.Helper()
	position := -1
	for _, value := range values {
		next := strings.Index(text[position+1:], value)
		if next < 0 {
			t.Fatalf("%q was not found after the previous value", value)
		}
		position += next + 1
	}
}

func TestRenderHTMLExportGroupsRootAndReplies(t *testing.T) {
	html := renderTestExport(t, []*model.Post{
		{Id: "reply-b", RootId: "root", UserId: "tester-id", CreateAt: 3_000, Message: "second reply"},
		{Id: "root", UserId: "master-id", CreateAt: 1_000, Message: "opening message"},
		{Id: "reply-a", RootId: "root", UserId: "tester-id", CreateAt: 2_000, Message: "first reply"},
	}, nil)

	if strings.Count(html, `class="thread"`) != 1 || strings.Count(html, `class="reply"`) != 2 {
		t.Fatalf("expected one thread with two nested replies:\n%s", html)
	}
	assertInOrder(t, html, "opening message", `class="replies"`, "first reply", "second reply")
}

func TestRenderHTMLExportSortsMixedThreadsAndReplies(t *testing.T) {
	html := renderTestExport(t, []*model.Post{
		{Id: "reply-b", RootId: "early-root", UserId: "tester-id", CreateAt: 5_000, Message: "same-time reply B"},
		{Id: "late-root", UserId: "master-id", CreateAt: 4_000, Message: "late thread"},
		{Id: "reply-a", RootId: "early-root", UserId: "tester-id", CreateAt: 5_000, Message: "same-time reply A"},
		{Id: "early-root", UserId: "master-id", CreateAt: 1_000, Message: "early thread"},
		{Id: "middle-root", UserId: "tester-id", CreateAt: 3_000, Message: "standalone middle"},
	}, nil)

	if strings.Count(html, `class="thread"`) != 3 {
		t.Fatalf("expected three threads:\n%s", html)
	}
	assertInOrder(t, html, "early thread", "same-time reply A", "same-time reply B", "standalone middle", "late thread")
}

func TestRenderHTMLExportUsesIDAsThreadTieBreaker(t *testing.T) {
	html := renderTestExport(t, []*model.Post{
		{Id: "root-b", UserId: "master-id", CreateAt: 1_000, Message: "thread B"},
		{Id: "root-a", UserId: "tester-id", CreateAt: 1_000, Message: "thread A"},
	}, nil)

	assertInOrder(t, html, "thread A", "thread B")
}

func TestRenderHTMLExportShowsStandaloneRootAsOrdinaryMessage(t *testing.T) {
	html := renderTestExport(t, []*model.Post{
		{Id: "only-root", UserId: "master-id", CreateAt: 1_000, Message: "just a message"},
	}, nil)

	if !strings.Contains(html, "just a message") || strings.Contains(html, `class="replies"`) {
		t.Fatalf("standalone message was not rendered plainly:\n%s", html)
	}
}

func TestRenderHTMLExportGroupsRepliesWhoseRootIsMissing(t *testing.T) {
	html := renderTestExport(t, []*model.Post{
		{Id: "orphan-2", RootId: "unavailable-secret-id", UserId: "tester-id", CreateAt: 3_000, Message: "later orphan"},
		{Id: "ordinary", UserId: "master-id", CreateAt: 2_000, Message: "ordinary thread"},
		{Id: "orphan-1", RootId: "unavailable-secret-id", UserId: "master-id", CreateAt: 1_000, Message: "earlier orphan"},
	}, nil)

	if strings.Count(html, "Earlier message is not included in this export") != 1 {
		t.Fatalf("expected one missing-root marker:\n%s", html)
	}
	assertInOrder(t, html, "Earlier message is not included", "earlier orphan", "later orphan", "ordinary thread")
	if strings.Contains(html, "unavailable-secret-id") {
		t.Fatalf("root ID leaked into output:\n%s", html)
	}
}

func TestRenderHTMLExportOmitsInternalMetadataAndOnlyShowsAttachmentName(t *testing.T) {
	html := renderTestExport(t, []*model.Post{
		{Id: "post-secret-123", RootId: "root-secret-456", UserId: "master-id", CreateAt: 1_000, Message: "attached"},
	}, map[string][]AttachmentMetadata{
		"post-secret-123": {{ID: "file-secret-789", Filename: "report.pdf", Size: 987654, MIMEType: "application/secret"}},
	})

	if !strings.Contains(html, "report.pdf") {
		t.Fatalf("attachment filename is missing:\n%s", html)
	}
	for _, forbidden := range []string{
		"post-secret-123", "root-secret-456", "file-secret-789", "987654", "application/secret",
		"Root post", "Reply to post", "Scope", "data-post-id", "RootId", "MIME type",
	} {
		if strings.Contains(html, forbidden) {
			t.Errorf("rendered HTML contains internal value %q", forbidden)
		}
	}
}

func TestRenderHTMLExportEscapesAllUserControlledFields(t *testing.T) {
	attack := `<script>alert("export")</script>`
	got, err := renderHTMLExport(
		&model.User{Id: "master-id", Username: attack},
		&model.User{Id: "tester-id", Username: `target&friend`},
		time.Unix(0, 0),
		defaultMaxExportPosts,
		[]*model.Post{{Id: "post", UserId: "master-id", CreateAt: 1_000, Message: attack + "\nsecond line"}},
		map[string][]AttachmentMetadata{"post": {{Filename: attack}}},
	)
	if err != nil {
		t.Fatalf("renderHTMLExport returned an error: %v", err)
	}

	html := string(got)
	if strings.Contains(html, attack) || strings.Contains(html, "<script>") {
		t.Fatalf("rendered HTML contains unescaped input: %s", html)
	}
	if strings.Count(html, "&lt;script&gt;") != 5 { // title, heading, author, message, and filename
		t.Fatalf("not every user field was escaped: %s", html)
	}
	if !strings.Contains(html, "target&amp;friend") || !strings.Contains(html, "\nsecond line") {
		t.Fatalf("participant escaping or message line break is missing: %s", html)
	}
}

func TestRenderHTMLExportHasFriendlyHeadingAndUTCDate(t *testing.T) {
	html := renderTestExport(t, []*model.Post{
		{Id: "post", UserId: "master-id", CreateAt: 1_700_000_000_000, Message: "hello"},
	}, nil)

	for _, required := range []string{
		"<title>Direct messages: @master and @tester</title>",
		"<h1>Direct messages: @master and @tester</h1>",
		"November 14, 2023 at 22:13:20 UTC",
		"Exported 1 messages. Configured limit: 1000.",
		"white-space: pre-wrap", "overflow-wrap: anywhere",
	} {
		if !strings.Contains(html, required) {
			t.Errorf("rendered HTML does not contain %q", required)
		}
	}
}

func TestRenderHTMLExportIgnoresNilPosts(t *testing.T) {
	html := renderTestExport(t, []*model.Post{
		nil,
		{Id: "post", UserId: "master-id", CreateAt: 1_000, Message: "survives"},
		nil,
	}, nil)

	if strings.Count(html, `class="thread"`) != 1 || !strings.Contains(html, "survives") {
		t.Fatalf("nil posts affected rendered output:\n%s", html)
	}
}

func TestRenderChannelHTMLExportUsesChannelTitleAndResolvedAuthors(t *testing.T) {
	posts := []*model.Post{
		{Id: "root", UserId: "first-id", CreateAt: 1_700_000_000_000, Message: "opening"},
		{Id: "reply", RootId: "root", UserId: "second-id", CreateAt: 1_700_000_001_000, Message: "response"},
	}
	got, err := renderChannelHTMLExport(
		&model.Channel{DisplayName: "Project <alpha> & friends", Name: "project-alpha"},
		map[string]string{"first-id": "@first", "second-id": "@second"},
		25,
		posts,
		map[string][]AttachmentMetadata{"reply": {{Filename: `notes <final>.txt`}}},
	)
	if err != nil {
		t.Fatalf("renderChannelHTMLExport returned an error: %v", err)
	}

	html := string(got)
	for _, required := range []string{
		"<title>Channel: Project &lt;alpha&gt; &amp; friends</title>",
		"<h1>Channel: Project &lt;alpha&gt; &amp; friends</h1>",
		"<strong>@first</strong>", "<strong>@second</strong>",
		"November 14, 2023 at 22:13:20 UTC", "notes &lt;final&gt;.txt",
		"Exported 2 messages. Configured limit: 25.",
	} {
		if !strings.Contains(html, required) {
			t.Errorf("rendered HTML does not contain %q", required)
		}
	}
	assertInOrder(t, html, "opening", `class="replies"`, "response")
}

func TestRenderChannelHTMLExportEscapesHostileContentAndUsesFallbackAuthors(t *testing.T) {
	attack := `<script>alert("channel")</script>`
	posts := []*model.Post{
		{Id: "unknown", UserId: "missing-id", CreateAt: 2_000, Message: attack},
		{Id: "system", CreateAt: 1_000, Message: "system message"},
	}
	channel := &model.Channel{DisplayName: attack, Name: "fallback-name"}

	first, err := renderChannelHTMLExport(channel, nil, 10, posts, nil)
	if err != nil {
		t.Fatalf("first render returned an error: %v", err)
	}
	second, err := renderChannelHTMLExport(channel, nil, 10, posts, nil)
	if err != nil {
		t.Fatalf("second render returned an error: %v", err)
	}
	if string(first) != string(second) {
		t.Fatal("identical channel export data produced different HTML")
	}

	html := string(first)
	if strings.Contains(html, "<script>") || strings.Contains(html, attack) {
		t.Fatalf("rendered HTML contains unescaped channel data: %s", html)
	}
	for _, required := range []string{"&lt;script&gt;", "<strong>System</strong>", "<strong>Unknown user</strong>"} {
		if !strings.Contains(html, required) {
			t.Errorf("rendered HTML does not contain %q", required)
		}
	}
	assertInOrder(t, html, "system message", "Unknown user", "&lt;script&gt;")
}

func TestRenderChannelHTMLExportFallsBackToChannelName(t *testing.T) {
	got, err := renderChannelHTMLExport(&model.Channel{Name: "town-square"}, nil, 10, nil, nil)
	if err != nil {
		t.Fatalf("renderChannelHTMLExport returned an error: %v", err)
	}
	if !strings.Contains(string(got), "<h1>Channel: town-square</h1>") {
		t.Fatalf("channel name was not used as the heading: %s", got)
	}
}
