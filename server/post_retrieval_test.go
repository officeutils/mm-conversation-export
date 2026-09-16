package main

import (
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
)

func TestExecuteCommandGetsLatestPostsOnce(t *testing.T) {
	posts := validPostGetter()

	response, appErr := (&Plugin{
		userGetter:    validUserGetter(),
		channelGetter: validChannelGetter(),
		memberGetter:  validMemberGetter(),
		postGetter:    posts,
	}).ExecuteCommand(nil, &model.CommandArgs{Command: "/export-dm @other", UserId: "requester-id"})
	if appErr != nil {
		t.Fatalf("ExecuteCommand returned an AppError: %v", appErr)
	}
	if response.Text != "Preparing a direct-message export with @other." {
		t.Fatalf("response text = %q", response.Text)
	}
	if posts.calls != 1 || posts.channelID != "direct-channel-id" || posts.page != 0 || posts.perPage != postLimit {
		t.Errorf("GetPostsForChannel calls = %d, args = (%q, %d, %d), want 1 call with (direct-channel-id, 0, %d)", posts.calls, posts.channelID, posts.page, posts.perPage, postLimit)
	}
}

func TestGetSortedChannelPostsSortsByCreateAtThenID(t *testing.T) {
	posts := &recordingPostGetter{postList: &model.PostList{
		Order: []string{"newer", "same-b", "same-a", "oldest"},
		Posts: map[string]*model.Post{
			"newer":  {Id: "newer", CreateAt: 30},
			"same-b": {Id: "b", CreateAt: 20},
			"same-a": {Id: "a", CreateAt: 20},
			"oldest": {Id: "oldest", CreateAt: 10},
		},
	}}

	got, appErr := getSortedChannelPosts(posts, "channel-id")
	if appErr != nil {
		t.Fatalf("getSortedChannelPosts returned an AppError: %v", appErr)
	}
	want := []string{"oldest", "a", "b", "newer"}
	if len(got) != len(want) {
		t.Fatalf("got %d posts, want %d", len(got), len(want))
	}
	for i, post := range got {
		if post.Id != want[i] {
			t.Errorf("post %d ID = %q, want %q", i, post.Id, want[i])
		}
	}
	if posts.calls != 1 || posts.channelID != "channel-id" || posts.page != 0 || posts.perPage != postLimit {
		t.Errorf("GetPostsForChannel calls = %d, args = (%q, %d, %d)", posts.calls, posts.channelID, posts.page, posts.perPage)
	}
}

func TestExecuteCommandHandlesPostLookupFailures(t *testing.T) {
	lookupError := model.NewAppError("test", "lookup failed", nil, "", 500)
	for _, tt := range []struct {
		name  string
		posts *recordingPostGetter
	}{
		{name: "lookup error", posts: &recordingPostGetter{err: lookupError}},
		{name: "nil post list", posts: &recordingPostGetter{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response, appErr := (&Plugin{
				userGetter:    validUserGetter(),
				channelGetter: validChannelGetter(),
				memberGetter:  validMemberGetter(),
				postGetter:    tt.posts,
			}).ExecuteCommand(nil, &model.CommandArgs{Command: "/export-dm @other", UserId: "requester-id"})
			if appErr != nil {
				t.Fatalf("ExecuteCommand returned an AppError: %v", appErr)
			}
			if response.Text != "Unable to read that direct-message conversation." {
				t.Errorf("response text = %q", response.Text)
			}
			if tt.posts.calls != 1 {
				t.Errorf("GetPostsForChannel calls = %d, want 1", tt.posts.calls)
			}
		})
	}
}
