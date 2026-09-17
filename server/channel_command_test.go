package main

import (
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
)

type recordingCurrentChannelGetter struct {
	channel            *model.Channel
	err                *model.AppError
	requestedChannelID string
	calls              int
}

func (g *recordingCurrentChannelGetter) GetChannel(channelID string) (*model.Channel, *model.AppError) {
	g.calls++
	g.requestedChannelID = channelID
	return g.channel, g.err
}

func TestExportChannelCommandAcceptsExactCommandAndCurrentContext(t *testing.T) {
	channels := &recordingCurrentChannelGetter{channel: &model.Channel{Id: "channel-id", Type: model.ChannelTypeOpen}}
	response, appErr := (&Plugin{
		currentChannelGetter: channels,
		memberGetter:         validChannelCommandMemberGetter(),
		permissionChecker:    &recordingChannelPermissionChecker{allowed: true},
	}).ExecuteCommand(nil, &model.CommandArgs{
		Command:   "/export-channel",
		UserId:    "requester-id",
		ChannelId: "channel-id",
	})

	if appErr != nil {
		t.Fatalf("ExecuteCommand returned an AppError: %v", appErr)
	}
	if response == nil || response.ResponseType != "ephemeral" || response.Text == "" {
		t.Fatalf("unexpected response: %#v", response)
	}
	if channels.calls != 1 || channels.requestedChannelID != "channel-id" {
		t.Errorf("GetChannel calls = %d with %q, want 1 with channel-id", channels.calls, channels.requestedChannelID)
	}
}

func TestExportChannelCommandRejectsInvalidParsingAndContextWithoutLookup(t *testing.T) {
	tests := []struct {
		name string
		args *model.CommandArgs
	}{
		{name: "argument", args: &model.CommandArgs{Command: "/export-channel extra", UserId: "requester-id", ChannelId: "channel-id"}},
		{name: "trailing whitespace", args: &model.CommandArgs{Command: "/export-channel ", UserId: "requester-id", ChannelId: "channel-id"}},
		{name: "missing user", args: &model.CommandArgs{Command: "/export-channel", ChannelId: "channel-id"}},
		{name: "missing channel", args: &model.CommandArgs{Command: "/export-channel", UserId: "requester-id"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channels := &recordingCurrentChannelGetter{}
			response, appErr := (&Plugin{currentChannelGetter: channels}).ExecuteCommand(nil, tt.args)
			if appErr != nil {
				t.Fatalf("ExecuteCommand returned an AppError: %v", appErr)
			}
			if response == nil || response.ResponseType != "ephemeral" || response.Text == "" {
				t.Fatalf("unexpected response: %#v", response)
			}
			if channels.calls != 0 {
				t.Errorf("GetChannel called %d times, want 0", channels.calls)
			}
		})
	}
}

func TestExportChannelCommandRejectsLookupAndIdentityFailures(t *testing.T) {
	lookupError := model.NewAppError("test", "lookup failed", nil, "", 500)
	tests := []struct {
		name    string
		channel *model.Channel
		err     *model.AppError
	}{
		{name: "lookup error", err: lookupError},
		{name: "nil channel"},
		{name: "mismatched ID", channel: &model.Channel{Id: "different-id", Type: model.ChannelTypeOpen}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertChannelCommandRejected(t, &recordingCurrentChannelGetter{channel: tt.channel, err: tt.err})
		})
	}
}

func TestExportChannelCommandAllowsOnlyActiveOpenOrPrivateChannels(t *testing.T) {
	tests := []struct {
		name    string
		type_   model.ChannelType
		deleted bool
		wantOK  bool
	}{
		{name: "open", type_: model.ChannelTypeOpen, wantOK: true},
		{name: "private", type_: model.ChannelTypePrivate, wantOK: true},
		{name: "direct", type_: model.ChannelTypeDirect},
		{name: "group", type_: model.ChannelTypeGroup},
		{name: "unknown", type_: model.ChannelType("x")},
		{name: "archived open", type_: model.ChannelTypeOpen, deleted: true},
		{name: "archived private", type_: model.ChannelTypePrivate, deleted: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deleteAt := int64(0)
			if tt.deleted {
				deleteAt = 1
			}
			channels := &recordingCurrentChannelGetter{channel: &model.Channel{Id: "channel-id", Type: tt.type_, DeleteAt: deleteAt}}
			response := executeChannelCommand(t, channels)
			gotOK := response.Text == "The current channel is eligible for export."
			if gotOK != tt.wantOK {
				t.Errorf("response text = %q, accepted = %t, want %t", response.Text, gotOK, tt.wantOK)
			}
		})
	}
}

func TestExportChannelCommandAuthorizesMembersWithReadPermission(t *testing.T) {
	for _, channelType := range []model.ChannelType{model.ChannelTypeOpen, model.ChannelTypePrivate} {
		t.Run(string(channelType), func(t *testing.T) {
			members := validChannelCommandMemberGetter()
			permissions := &recordingChannelPermissionChecker{allowed: true}
			response, appErr := (&Plugin{
				currentChannelGetter: &recordingCurrentChannelGetter{channel: &model.Channel{Id: "channel-id", Type: channelType}},
				memberGetter:         members,
				permissionChecker:    permissions,
			}).ExecuteCommand(nil, channelCommandArgs())
			if appErr != nil {
				t.Fatalf("ExecuteCommand returned an AppError: %v", appErr)
			}
			if response.Text != "The current channel is eligible for export." {
				t.Fatalf("response text = %q", response.Text)
			}
			if len(members.calls) != 1 || members.calls[0] != "requester-id" {
				t.Errorf("GetChannelMember calls = %v, want [requester-id]", members.calls)
			}
			if len(members.memberChannelIDs) != 1 || members.memberChannelIDs[0] != "channel-id" {
				t.Errorf("GetChannelMember channel IDs = %v, want [channel-id]", members.memberChannelIDs)
			}
			if members.channelID != "" {
				t.Errorf("GetChannelMembers was called for %q", members.channelID)
			}
			if permissions.calls != 1 || permissions.userID != "requester-id" || permissions.channelID != "channel-id" || permissions.permission != model.PermissionReadChannel {
				t.Errorf("HasPermissionToChannel calls/args = %d, %q, %q, %v", permissions.calls, permissions.userID, permissions.channelID, permissions.permission)
			}
		})
	}
}

func TestExportChannelCommandRejectsFailedAuthorizationBeforeContentAccess(t *testing.T) {
	lookupError := model.NewAppError("test", "lookup failed", nil, "", 500)
	notFoundError := model.NewAppError("test", "membership not found", nil, "", 404)
	tests := []struct {
		name               string
		channelType        model.ChannelType
		member             *model.ChannelMember
		memberErr          *model.AppError
		permissionAllowed  bool
		wantPermissionCall bool
	}{
		{name: "absent membership in private channel", channelType: model.ChannelTypePrivate, memberErr: notFoundError, permissionAllowed: true},
		{name: "failed membership lookup", channelType: model.ChannelTypePrivate, memberErr: lookupError, permissionAllowed: true},
		{name: "nil membership in public channel", channelType: model.ChannelTypeOpen, permissionAllowed: true},
		{name: "membership has mismatched channel", channelType: model.ChannelTypeOpen, member: &model.ChannelMember{ChannelId: "other-channel", UserId: "requester-id"}, permissionAllowed: true},
		{name: "membership has mismatched user", channelType: model.ChannelTypePrivate, member: &model.ChannelMember{ChannelId: "channel-id", UserId: "other-user"}, permissionAllowed: true},
		{name: "read permission denied", channelType: model.ChannelTypePrivate, member: &model.ChannelMember{ChannelId: "channel-id", UserId: "requester-id"}, wantPermissionCall: true},
		{name: "administrator without membership", channelType: model.ChannelTypePrivate, permissionAllowed: true},
		{name: "guest cannot rely on public channel visibility", channelType: model.ChannelTypeOpen, permissionAllowed: true},
		{name: "guest public member denied read permission", channelType: model.ChannelTypeOpen, member: &model.ChannelMember{ChannelId: "channel-id", UserId: "requester-id"}, wantPermissionCall: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			members := &memberLookup{
				members:      map[string]*model.ChannelMember{"requester-id": tt.member},
				memberErrors: map[string]*model.AppError{"requester-id": tt.memberErr},
			}
			permissions := &recordingChannelPermissionChecker{allowed: tt.permissionAllowed}
			posts := validPostGetter()
			files := &recordingFileInfoGetter{}
			response, appErr := (&Plugin{
				currentChannelGetter: &recordingCurrentChannelGetter{channel: &model.Channel{Id: "channel-id", Type: tt.channelType}},
				memberGetter:         members,
				permissionChecker:    permissions,
				postGetter:           posts,
				fileGetter:           files,
			}).ExecuteCommand(nil, channelCommandArgs())
			if appErr != nil {
				t.Fatalf("ExecuteCommand returned an AppError: %v", appErr)
			}
			if response.Text != "Unable to export the current channel." {
				t.Errorf("response text = %q, want non-disclosing rejection", response.Text)
			}
			wantPermissionCalls := 0
			if tt.wantPermissionCall {
				wantPermissionCalls = 1
			}
			if permissions.calls != wantPermissionCalls {
				t.Errorf("HasPermissionToChannel calls = %d, want %d", permissions.calls, wantPermissionCalls)
			}
			if members.channelID != "" {
				t.Errorf("GetChannelMembers was called for %q", members.channelID)
			}
			if posts.calls != 0 {
				t.Errorf("GetPostsForChannel calls = %d, want 0", posts.calls)
			}
			if len(files.calls) != 0 {
				t.Errorf("GetFileInfo calls = %v, want none", files.calls)
			}
		})
	}
}

func assertChannelCommandRejected(t *testing.T, channels *recordingCurrentChannelGetter) {
	t.Helper()
	response := executeChannelCommand(t, channels)
	if response.Text != "Unable to export the current channel." {
		t.Errorf("response text = %q, want non-disclosing rejection", response.Text)
	}
}

func executeChannelCommand(t *testing.T, channels *recordingCurrentChannelGetter) *model.CommandResponse {
	t.Helper()
	response, appErr := (&Plugin{
		currentChannelGetter: channels,
		memberGetter:         validChannelCommandMemberGetter(),
		permissionChecker:    &recordingChannelPermissionChecker{allowed: true},
	}).ExecuteCommand(nil, &model.CommandArgs{
		Command:   "/export-channel",
		UserId:    "requester-id",
		ChannelId: "channel-id",
	})
	if appErr != nil {
		t.Fatalf("ExecuteCommand returned an AppError: %v", appErr)
	}
	if response == nil || response.ResponseType != "ephemeral" {
		t.Fatalf("unexpected response: %#v", response)
	}
	return response
}

func validChannelCommandMemberGetter() *memberLookup {
	return &memberLookup{
		members: map[string]*model.ChannelMember{
			"requester-id": {ChannelId: "channel-id", UserId: "requester-id"},
		},
		memberErrors: map[string]*model.AppError{},
	}
}

func channelCommandArgs() *model.CommandArgs {
	return &model.CommandArgs{Command: "/export-channel", UserId: "requester-id", ChannelId: "channel-id"}
}
