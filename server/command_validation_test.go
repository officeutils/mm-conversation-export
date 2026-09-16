package main

import (
	"strings"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
)

func TestExecuteCommandAcceptsExactlyOneUsername(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
	}{
		{name: "plain username", command: "/export-dm other-user", want: "Preparing a direct-message export with @other-user."},
		{name: "leading at sign", command: "/export-dm @other.user", want: "Preparing a direct-message export with @other.user."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			users := validUserGetter()
			channels := validChannelGetter()
			response, appErr := (&Plugin{userGetter: users, channelGetter: channels, memberGetter: validMemberGetter(), postGetter: validPostGetter()}).ExecuteCommand(nil, &model.CommandArgs{
				Command: tt.command,
				UserId:  "requester-id",
			})
			if appErr != nil {
				t.Fatalf("ExecuteCommand returned an AppError: %v", appErr)
			}
			if response.ResponseType != "ephemeral" {
				t.Errorf("response type = %q, want ephemeral", response.ResponseType)
			}
			if response.Text != tt.want {
				t.Errorf("response text = %q, want %q", response.Text, tt.want)
			}
			if users.requestedUserID != "requester-id" {
				t.Errorf("GetUser called with %q, want requester-id", users.requestedUserID)
			}
			if users.requestedUsername != strings.TrimPrefix(strings.Fields(tt.command)[1], "@") {
				t.Errorf("GetUserByUsername called with %q", users.requestedUsername)
			}
			if channels.calls != 1 || channels.teamID != "" || channels.userID != "requester-id" || channels.includeDeleted {
				t.Errorf("GetChannelsForTeamForUser calls = %d, args = (%q, %q, %t), want 1 call with (\"\", \"requester-id\", false)", channels.calls, channels.teamID, channels.userID, channels.includeDeleted)
			}
		})
	}
}

func TestExecuteCommandRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name string
		args *model.CommandArgs
	}{
		{name: "nil arguments"},
		{name: "missing requester", args: &model.CommandArgs{Command: "/export-dm @other"}},
		{name: "missing username", args: &model.CommandArgs{Command: "/export-dm", UserId: "requester-id"}},
		{name: "additional argument", args: &model.CommandArgs{Command: "/export-dm other extra", UserId: "requester-id"}},
		{name: "only at sign", args: &model.CommandArgs{Command: "/export-dm @", UserId: "requester-id"}},
		{name: "two at signs", args: &model.CommandArgs{Command: "/export-dm @@other", UserId: "requester-id"}},
		{name: "uppercase username", args: &model.CommandArgs{Command: "/export-dm Other", UserId: "requester-id"}},
		{name: "invalid character", args: &model.CommandArgs{Command: "/export-dm other/user", UserId: "requester-id"}},
		{name: "wrong command", args: &model.CommandArgs{Command: "/something-else other", UserId: "requester-id"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, appErr := (&Plugin{userGetter: validUserGetter(), channelGetter: validChannelGetter()}).ExecuteCommand(nil, tt.args)
			if appErr != nil {
				t.Fatalf("ExecuteCommand returned an AppError: %v", appErr)
			}
			if response == nil {
				t.Fatal("ExecuteCommand returned a nil response")
			}
			if response.ResponseType != "ephemeral" {
				t.Errorf("response type = %q, want ephemeral", response.ResponseType)
			}
			if response.Text == "" {
				t.Error("response did not explain the rejection")
			}
		})
	}
}

func TestExecuteCommandHandlesUserLookupFailures(t *testing.T) {
	lookupError := model.NewAppError("test", "lookup failed", nil, "", 500)
	tests := []struct {
		name  string
		users *recordingUserGetter
		want  string
	}{
		{
			name:  "requester lookup error",
			users: &recordingUserGetter{requesterErr: lookupError},
			want:  "Unable to resolve the authenticated requester.",
		},
		{
			name:  "nil requester",
			users: &recordingUserGetter{},
			want:  "Unable to resolve the authenticated requester.",
		},
		{
			name: "target lookup error",
			users: &recordingUserGetter{
				requester: &model.User{Id: "requester-id"},
				targetErr: lookupError,
			},
			want: "Unable to find user @other.",
		},
		{
			name:  "nil target",
			users: &recordingUserGetter{requester: &model.User{Id: "requester-id"}},
			want:  "Unable to find user @other.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, appErr := (&Plugin{userGetter: tt.users, channelGetter: validChannelGetter()}).ExecuteCommand(nil, &model.CommandArgs{
				Command: "/export-dm @other",
				UserId:  "requester-id",
			})
			if appErr != nil {
				t.Fatalf("ExecuteCommand returned an AppError: %v", appErr)
			}
			if response.Text != tt.want {
				t.Errorf("response text = %q, want %q", response.Text, tt.want)
			}
		})
	}
}

func TestExecuteCommandRejectsRequesterAsTarget(t *testing.T) {
	users := validUserGetter()
	users.target.Id = users.requester.Id

	response, appErr := (&Plugin{userGetter: users, channelGetter: validChannelGetter()}).ExecuteCommand(nil, &model.CommandArgs{
		Command: "/export-dm @requester",
		UserId:  "requester-id",
	})
	if appErr != nil {
		t.Fatalf("ExecuteCommand returned an AppError: %v", appErr)
	}
	if response.Text != "You cannot export a direct-message conversation with yourself." {
		t.Errorf("unexpected response text: %q", response.Text)
	}
}
