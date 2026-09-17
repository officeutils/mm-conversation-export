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
	response, appErr := (&Plugin{currentChannelGetter: channels}).ExecuteCommand(nil, &model.CommandArgs{
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

func assertChannelCommandRejected(t *testing.T, channels *recordingCurrentChannelGetter) {
	t.Helper()
	response := executeChannelCommand(t, channels)
	if response.Text != "Unable to export the current channel." {
		t.Errorf("response text = %q, want non-disclosing rejection", response.Text)
	}
}

func executeChannelCommand(t *testing.T, channels *recordingCurrentChannelGetter) *model.CommandResponse {
	t.Helper()
	response, appErr := (&Plugin{currentChannelGetter: channels}).ExecuteCommand(nil, &model.CommandArgs{
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
