//go:build integration

package main

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
)

// TestMattermost10112ExportChannelReceivesCurrentPublicAndPrivateChannel
// exercises the command through Mattermost itself. A successful export proves
// that Mattermost populated CommandArgs.ChannelId with the channel in which the
// command was invoked: the plugin compares that ID with GetChannel, membership,
// and PermissionReadChannel before reading any history.
//
// The plugin must already be installed with EnableChannelExport enabled. The
// configured user must be a member of both configured channels.
func TestMattermost10112ExportChannelReceivesCurrentPublicAndPrivateChannel(t *testing.T) {
	serverURL := integrationEnv(t, "MM_INTEGRATION_URL")
	username := integrationEnv(t, "MM_INTEGRATION_USERNAME")
	password := integrationEnv(t, "MM_INTEGRATION_PASSWORD")

	ctx := context.Background()
	client, user := loginIntegrationUser(t, ctx, serverURL, username, password)
	tests := []struct {
		name        string
		env         string
		channelType model.ChannelType
	}{
		{name: "public", env: "MM_INTEGRATION_PUBLIC_CHANNEL_ID", channelType: model.ChannelTypeOpen},
		{name: "private", env: "MM_INTEGRATION_PRIVATE_CHANNEL_ID", channelType: model.ChannelTypePrivate},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertIntegrationChannelExport(t, ctx, client, user, integrationEnv(t, tt.env), tt.channelType)
		})
	}
}

// TestMattermost10112GuestCanExportJoinedPublicChannel protects the two checks
// required for guest access: explicit public-channel membership and the
// effective PermissionReadChannel permission. A public channel's visibility is
// not treated as a substitute for either check.
func TestMattermost10112GuestCanExportJoinedPublicChannel(t *testing.T) {
	serverURL := integrationEnv(t, "MM_INTEGRATION_URL")
	username := integrationEnv(t, "MM_INTEGRATION_GUEST_USERNAME")
	password := integrationEnv(t, "MM_INTEGRATION_GUEST_PASSWORD")
	channelID := integrationEnv(t, "MM_INTEGRATION_GUEST_PUBLIC_CHANNEL_ID")

	ctx := context.Background()
	client, guest := loginIntegrationUser(t, ctx, serverURL, username, password)
	if !strings.Contains(guest.Roles, "system_guest") {
		t.Fatalf("configured guest %q has roles %q, want system_guest", username, guest.Roles)
	}
	assertIntegrationChannelExport(t, ctx, client, guest, channelID, model.ChannelTypeOpen)
}

func assertIntegrationChannelExport(t *testing.T, ctx context.Context, client *model.Client4, user *model.User, channelID string, wantType model.ChannelType) {
	t.Helper()

	channel, _, err := client.GetChannel(ctx, channelID, "")
	if err != nil {
		t.Fatalf("get configured channel %s: %v", channelID, err)
	}
	if channel.Id != channelID || channel.Type != wantType {
		t.Fatalf("configured channel = (%q, %q), want (%q, %q)", channel.Id, channel.Type, channelID, wantType)
	}
	member, _, err := client.GetChannelMember(ctx, channelID, user.Id, "")
	if err != nil {
		t.Fatalf("get configured channel membership: %v", err)
	}
	if member.ChannelId != channelID || member.UserId != user.Id {
		t.Fatalf("membership = (%q, %q), want (%q, %q)", member.ChannelId, member.UserId, channelID, user.Id)
	}

	response, _, err := client.ExecuteCommand(ctx, channelID, "/export-channel")
	if err != nil {
		t.Fatalf("execute /export-channel in %s: %v", channelID, err)
	}
	if response == nil || response.ResponseType != "ephemeral" || !strings.HasPrefix(response.Text, "[Download your channel export]") {
		t.Fatalf("command response = %#v; verify EnableChannelExport is enabled and PermissionReadChannel is granted", response)
	}
	start := strings.Index(response.Text, "](")
	end := strings.Index(response.Text, ").")
	if start < 0 || end <= start+2 {
		t.Fatalf("command response has no download URL: %q", response.Text)
	}
	download, err := client.DoAPIRequest(ctx, http.MethodGet, client.URL+response.Text[start+2:end], "", "")
	if err != nil {
		t.Fatalf("consume export download: %v", err)
	}
	download.Body.Close()
}
