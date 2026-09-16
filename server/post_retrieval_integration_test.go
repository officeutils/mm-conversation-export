//go:build integration

package main

import (
	"context"
	"os"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
)

const mattermostRegressionVersion = "10.11.2"

// TestMattermost10112BoundedChannelPostsIncludeThreadReply protects the
// server behavior on which getSortedChannelPosts relies. In Mattermost
// 10.11.2, the uncollapsed, bounded channel-post query includes both thread
// roots and replies; fetching each thread separately is therefore unnecessary.
func TestMattermost10112BoundedChannelPostsIncludeThreadReply(t *testing.T) {
	serverURL := integrationEnv(t, "MM_INTEGRATION_URL")
	username := integrationEnv(t, "MM_INTEGRATION_USERNAME")
	password := integrationEnv(t, "MM_INTEGRATION_PASSWORD")
	peerUsername := integrationEnv(t, "MM_INTEGRATION_PEER_USERNAME")
	peerPassword := integrationEnv(t, "MM_INTEGRATION_PEER_PASSWORD")

	ctx := context.Background()
	client, user := loginIntegrationUser(t, ctx, serverURL, username, password)
	peerClient, peer := loginIntegrationUser(t, ctx, serverURL, peerUsername, peerPassword)

	channel, _, err := client.CreateDirectChannel(ctx, user.Id, peer.Id)
	if err != nil {
		t.Fatalf("create direct channel: %v", err)
	}

	root, _, err := client.CreatePost(ctx, &model.Post{
		ChannelId: channel.Id,
		Message:   "10.11.2 bounded-post regression root " + model.NewId(),
	})
	if err != nil {
		t.Fatalf("create thread root: %v", err)
	}
	t.Cleanup(func() {
		if _, cleanupErr := client.DeletePost(context.Background(), root.Id); cleanupErr != nil {
			t.Logf("delete thread root %s: %v", root.Id, cleanupErr)
		}
	})

	reply, _, err := peerClient.CreatePost(ctx, &model.Post{
		ChannelId: channel.Id,
		RootId:    root.Id,
		Message:   "10.11.2 bounded-post regression reply " + model.NewId(),
	})
	if err != nil {
		t.Fatalf("create thread reply: %v", err)
	}
	t.Cleanup(func() {
		if _, cleanupErr := peerClient.DeletePost(context.Background(), reply.Id); cleanupErr != nil {
			t.Logf("delete thread reply %s: %v", reply.Id, cleanupErr)
		}
	})

	postList, _, err := client.GetPostsForChannel(ctx, channel.Id, 0, postLimit, "", false, false)
	if err != nil {
		t.Fatalf("get bounded, uncollapsed channel posts: %v", err)
	}
	if postList == nil {
		t.Fatal("bounded query returned a nil post list")
	}
	if len(postList.Order) > postLimit {
		t.Fatalf("bounded query returned %d posts, want at most %d", len(postList.Order), postLimit)
	}
	assertPostListContains(t, postList, root.Id)
	assertPostListContains(t, postList, reply.Id)
	if got := postList.Posts[reply.Id].RootId; got != root.Id {
		t.Fatalf("reply RootId = %q, want %q", got, root.Id)
	}
}

func loginIntegrationUser(t *testing.T, ctx context.Context, serverURL, username, password string) (*model.Client4, *model.User) {
	t.Helper()

	client := model.NewAPIv4Client(serverURL)
	user, response, err := client.Login(ctx, username, password)
	if err != nil {
		t.Fatalf("log in %q: %v", username, err)
	}
	if response.ServerVersion != mattermostRegressionVersion {
		t.Fatalf("Mattermost version = %q, test requires %s", response.ServerVersion, mattermostRegressionVersion)
	}
	return client, user
}

func integrationEnv(t *testing.T, name string) string {
	t.Helper()

	value := os.Getenv(name)
	if value == "" {
		t.Skipf("set %s to run the Mattermost %s integration test", name, mattermostRegressionVersion)
	}
	return value
}

func assertPostListContains(t *testing.T, postList *model.PostList, postID string) {
	t.Helper()

	if postList.Posts[postID] == nil {
		t.Fatalf("bounded query Posts does not contain %s", postID)
	}
	for _, orderedID := range postList.Order {
		if orderedID == postID {
			return
		}
	}
	t.Fatalf("bounded query Order does not contain %s", postID)
}
