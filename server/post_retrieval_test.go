package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

func postPage(ids ...string) *model.PostList {
	list := model.NewPostList()
	for i, id := range ids {
		list.Order = append(list.Order, id)
		list.Posts[id] = &model.Post{Id: id, CreateAt: int64(i)}
	}
	return list
}

func fullPostPage(prefix string, count int) *model.PostList {
	ids := make([]string, count)
	for i := range ids {
		ids[i] = fmt.Sprintf("%s-%03d", prefix, i)
	}
	return postPage(ids...)
}

func TestMaxExportPostsDefaultsAndValidates(t *testing.T) {
	p := &Plugin{}
	if got := p.maxExportPosts(); got != 1000 {
		t.Errorf("default = %d, want 1000", got)
	}
	p.configuration.MaxExportPosts = "37"
	if got := p.maxExportPosts(); got != 37 {
		t.Errorf("custom = %d, want 37", got)
	}
	for _, invalid := range []string{"invalid", "0", "-1", "10001"} {
		p.configuration.MaxExportPosts = invalid
		if got := p.maxExportPosts(); got != defaultMaxExportPosts {
			t.Errorf("invalid %q normalized to %d", invalid, got)
		}
		if _, err := parseMaxExportPosts(invalid); err == nil {
			t.Errorf("parseMaxExportPosts(%q) succeeded", invalid)
		}
	}
}

func TestGetSortedChannelPostsSingleAndIncompletePage(t *testing.T) {
	g := &recordingPostGetter{postList: postPage("b", "a")}
	got, err := getSortedChannelPosts(g, "channel", 1000)
	if err != nil || len(got) != 2 || g.calls != 1 {
		t.Fatalf("got %d posts, %d calls, err %v", len(got), g.calls, err)
	}
	if g.perPage != postPageSize {
		t.Errorf("page size = %d, want %d", g.perPage, postPageSize)
	}
}

func TestGetSortedChannelPostsMultiplePagesStopsAtLimitAndDeduplicates(t *testing.T) {
	first := fullPostPage("first", postPageSize)
	second := fullPostPage("second", postPageSize)
	// Simulate an item repeated at a page boundary.
	second.Order[0] = first.Order[len(first.Order)-1]
	second.Posts[second.Order[0]] = first.Posts[second.Order[0]]
	third := postPage("final")
	g := &recordingPostGetter{postLists: []*model.PostList{first, second, third}}
	got, err := getSortedChannelPosts(g, "channel", 250)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 250 {
		t.Fatalf("got %d posts, want exactly 250", len(got))
	}
	if g.calls != 2 {
		t.Errorf("calls = %d, want 2 (no unnecessary third page)", g.calls)
	}
	seen := map[string]bool{}
	for _, post := range got {
		if seen[post.Id] {
			t.Errorf("duplicate post %q", post.Id)
		}
		seen[post.Id] = true
	}
}

func TestGetSortedChannelPostsStopsAfterIncompleteFinalPage(t *testing.T) {
	g := &recordingPostGetter{postLists: []*model.PostList{fullPostPage("first", postPageSize), postPage("last-a", "last-b")}}
	got, err := getSortedChannelPosts(g, "channel", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 202 || g.calls != 2 {
		t.Errorf("got %d posts from %d calls, want 202 from 2", len(got), g.calls)
	}
}

func TestExecuteCommandUsesConfiguredLimitAndTimestampedFilename(t *testing.T) {
	posts := &recordingPostGetter{postList: postPage("one")}
	store := validExportStore()
	now := time.Date(2026, time.September, 16, 21, 30, 45, 0, time.FixedZone("offset", 3600))
	p := &Plugin{userGetter: validUserGetter(), channelGetter: validChannelGetter(), memberGetter: validMemberGetter(), postGetter: posts, exportStore: store, now: func() time.Time { return now }}
	p.configuration.MaxExportPosts = "37"
	response, appErr := p.ExecuteCommand(nil, &model.CommandArgs{Command: "/export-dm @other", UserId: "requester-id"})
	if appErr != nil || response.Text == "" {
		t.Fatalf("ExecuteCommand failed: %#v / %v", response, appErr)
	}
	if posts.perPage != 37 {
		t.Errorf("page size = %d, want configured limit 37", posts.perPage)
	}
	if store.filename != "dm-requester-other-2026-09-16-203045.html" {
		t.Errorf("filename = %q", store.filename)
	}
	if !strings.Contains(string(store.contents), "Exported 1 messages. Configured limit: 37.") {
		t.Error("configured/actual notice missing")
	}
}

func TestExportFilenameSanitizesComponents(t *testing.T) {
	got := exportFilename("../master/path", `target\\name`, time.Date(2026, 9, 16, 21, 30, 45, 0, time.UTC))
	if got != "dm-master-path-target-name-2026-09-16-213045.html" {
		t.Errorf("filename = %q", got)
	}
}

func TestGetSortedChannelPostsOrdersChronologicallyAndKeepsReplies(t *testing.T) {
	list := &model.PostList{Order: []string{"reply", "b", "a"}, Posts: map[string]*model.Post{
		"reply": {Id: "reply", RootId: "a", CreateAt: 30},
		"b":     {Id: "b", CreateAt: 20},
		"a":     {Id: "a", CreateAt: 20},
	}}
	got, err := getSortedChannelPosts(&recordingPostGetter{postList: list}, "channel", 10)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"a", "b", "reply"} {
		if got[i].Id != want {
			t.Errorf("post %d = %q, want %q", i, got[i].Id, want)
		}
	}
	if got[2].RootId != "a" {
		t.Error("thread reply metadata was not retained")
	}
}

func TestExecuteCommandHandlesPostLookupFailures(t *testing.T) {
	lookupError := model.NewAppError("test", "lookup failed", nil, "", 500)
	for _, posts := range []*recordingPostGetter{{err: lookupError}, {}} {
		response, appErr := (&Plugin{userGetter: validUserGetter(), channelGetter: validChannelGetter(), memberGetter: validMemberGetter(), postGetter: posts}).ExecuteCommand(nil, &model.CommandArgs{Command: "/export-dm @other", UserId: "requester-id"})
		if appErr != nil || response.Text != "Unable to read that direct-message conversation." {
			t.Errorf("unexpected response/error: %#v / %v", response, appErr)
		}
	}
}
