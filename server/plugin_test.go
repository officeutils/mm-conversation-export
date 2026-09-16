package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
)

type recordingRegistrar struct {
	command *model.Command
	err     error
}

type recordingUserGetter struct {
	requester         *model.User
	requesterErr      *model.AppError
	target            *model.User
	targetErr         *model.AppError
	requestedUserID   string
	requestedUsername string
}

type recordingChannelGetter struct {
	channels       []*model.Channel
	err            *model.AppError
	teamID         string
	userID         string
	includeDeleted bool
	calls          int
}

type memberLookup struct {
	members      map[string]*model.ChannelMember
	memberErrors map[string]*model.AppError
	allMembers   model.ChannelMembers
	allErr       *model.AppError
	calls        []string
	channelID    string
	page         int
	perPage      int
}

func (g *memberLookup) GetChannelMember(channelID, userID string) (*model.ChannelMember, *model.AppError) {
	g.calls = append(g.calls, userID)
	return g.members[userID], g.memberErrors[userID]
}

func (g *memberLookup) GetChannelMembers(channelID string, page, perPage int) (model.ChannelMembers, *model.AppError) {
	g.channelID = channelID
	g.page = page
	g.perPage = perPage
	return g.allMembers, g.allErr
}

func (g *recordingChannelGetter) GetChannelsForTeamForUser(teamID, userID string, includeDeleted bool) ([]*model.Channel, *model.AppError) {
	g.teamID = teamID
	g.userID = userID
	g.includeDeleted = includeDeleted
	g.calls++
	return g.channels, g.err
}

func (g *recordingUserGetter) GetUser(userID string) (*model.User, *model.AppError) {
	g.requestedUserID = userID
	return g.requester, g.requesterErr
}

func (g *recordingUserGetter) GetUserByUsername(username string) (*model.User, *model.AppError) {
	g.requestedUsername = username
	return g.target, g.targetErr
}

func validUserGetter() *recordingUserGetter {
	return &recordingUserGetter{
		requester: &model.User{Id: "requester-id", Username: "requester"},
		target:    &model.User{Id: "target-id", Username: "other"},
	}
}

func validChannelGetter() *recordingChannelGetter {
	return &recordingChannelGetter{channels: []*model.Channel{{
		Id:   "direct-channel-id",
		Name: model.GetDMNameFromIds("requester-id", "target-id"),
		Type: model.ChannelTypeDirect,
	}}}
}

func validMemberGetter() *memberLookup {
	requester := &model.ChannelMember{ChannelId: "direct-channel-id", UserId: "requester-id"}
	target := &model.ChannelMember{ChannelId: "direct-channel-id", UserId: "target-id"}
	return &memberLookup{
		members: map[string]*model.ChannelMember{
			"requester-id": requester,
			"target-id":    target,
		},
		memberErrors: map[string]*model.AppError{},
		allMembers:   model.ChannelMembers{requester, target},
	}
}

func (r *recordingRegistrar) RegisterCommand(command *model.Command) error {
	r.command = command
	return r.err
}

func TestOnActivateRegistersExportDMCommand(t *testing.T) {
	registrar := &recordingRegistrar{}
	p := &Plugin{commandRegistrar: registrar}

	if err := p.OnActivate(); err != nil {
		t.Fatalf("OnActivate returned an error: %v", err)
	}

	if registrar.command == nil {
		t.Fatal("OnActivate did not register a command")
	}
	if registrar.command.Trigger != commandTrigger {
		t.Errorf("command trigger = %q, want %q", registrar.command.Trigger, commandTrigger)
	}
	if !registrar.command.AutoComplete {
		t.Error("command autocomplete is disabled")
	}
	if registrar.command.AutoCompleteHint != "@username" {
		t.Errorf("command autocomplete hint = %q, want %q", registrar.command.AutoCompleteHint, "@username")
	}
}

func TestOnActivateReturnsRegistrationError(t *testing.T) {
	want := errors.New("registration failed")
	p := &Plugin{commandRegistrar: &recordingRegistrar{err: want}}

	if got := p.OnActivate(); !errors.Is(got, want) {
		t.Fatalf("OnActivate error = %v, want %v", got, want)
	}
}

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
			response, appErr := (&Plugin{userGetter: users, channelGetter: channels, memberGetter: validMemberGetter()}).ExecuteCommand(nil, &model.CommandArgs{
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

func TestExecuteCommandFindsExistingDirectChannel(t *testing.T) {
	channels := &recordingChannelGetter{channels: []*model.Channel{
		nil,
		{Id: "public", Name: model.GetDMNameFromIds("requester-id", "target-id"), Type: model.ChannelTypeOpen},
		{Id: "group", Name: model.GetDMNameFromIds("requester-id", "target-id"), Type: model.ChannelTypeGroup},
		{Id: "different-dm", Name: model.GetDMNameFromIds("requester-id", "someone-else"), Type: model.ChannelTypeDirect},
		{Id: "direct-channel-id", Name: model.GetDMNameFromIds("target-id", "requester-id"), Type: model.ChannelTypeDirect},
	}}

	response, appErr := (&Plugin{userGetter: validUserGetter(), channelGetter: channels, memberGetter: validMemberGetter()}).ExecuteCommand(nil, &model.CommandArgs{
		Command: "/export-dm @other",
		UserId:  "requester-id",
	})
	if appErr != nil {
		t.Fatalf("ExecuteCommand returned an AppError: %v", appErr)
	}
	if response.Text != "Preparing a direct-message export with @other." {
		t.Errorf("response text = %q", response.Text)
	}
}

func TestExecuteCommandVerifiesBothMembershipsAndParticipantSet(t *testing.T) {
	members := validMemberGetter()

	response, appErr := (&Plugin{
		userGetter:    validUserGetter(),
		channelGetter: validChannelGetter(),
		memberGetter:  members,
	}).ExecuteCommand(nil, &model.CommandArgs{Command: "/export-dm @other", UserId: "requester-id"})
	if appErr != nil {
		t.Fatalf("ExecuteCommand returned an AppError: %v", appErr)
	}
	if response.Text != "Preparing a direct-message export with @other." {
		t.Fatalf("response text = %q", response.Text)
	}
	if len(members.calls) != 2 || members.calls[0] != "requester-id" || members.calls[1] != "target-id" {
		t.Errorf("GetChannelMember calls = %v, want [requester-id target-id]", members.calls)
	}
	if members.channelID != "direct-channel-id" || members.page != 0 || members.perPage != 3 {
		t.Errorf("GetChannelMembers args = (%q, %d, %d), want (direct-channel-id, 0, 3)", members.channelID, members.page, members.perPage)
	}
}

func TestExecuteCommandRejectsUnauthorizedDirectChannels(t *testing.T) {
	lookupError := model.NewAppError("test", "lookup failed", nil, "", 500)
	tests := []struct {
		name      string
		mutate    func(*memberLookup)
		wantCalls []string
	}{
		{name: "requester membership lookup error", mutate: func(g *memberLookup) { g.memberErrors["requester-id"] = lookupError }, wantCalls: []string{"requester-id"}},
		{name: "requester membership absent", mutate: func(g *memberLookup) { g.members["requester-id"] = nil }, wantCalls: []string{"requester-id"}},
		{name: "requester membership mismatched", mutate: func(g *memberLookup) {
			g.members["requester-id"] = &model.ChannelMember{ChannelId: "other-channel", UserId: "requester-id"}
		}, wantCalls: []string{"requester-id"}},
		{name: "target membership lookup error", mutate: func(g *memberLookup) { g.memberErrors["target-id"] = lookupError }, wantCalls: []string{"requester-id", "target-id"}},
		{name: "target membership absent", mutate: func(g *memberLookup) { g.members["target-id"] = nil }, wantCalls: []string{"requester-id", "target-id"}},
		{name: "target membership mismatched", mutate: func(g *memberLookup) {
			g.members["target-id"] = &model.ChannelMember{ChannelId: "direct-channel-id", UserId: "someone-else"}
		}, wantCalls: []string{"requester-id", "target-id"}},
		{name: "participant enumeration error", mutate: func(g *memberLookup) { g.allErr = lookupError }, wantCalls: []string{"requester-id", "target-id"}},
		{name: "participant missing", mutate: func(g *memberLookup) { g.allMembers = g.allMembers[:1] }, wantCalls: []string{"requester-id", "target-id"}},
		{name: "unexpected third participant", mutate: func(g *memberLookup) {
			g.allMembers = append(g.allMembers, &model.ChannelMember{ChannelId: "direct-channel-id", UserId: "intruder-id"})
		}, wantCalls: []string{"requester-id", "target-id"}},
		{name: "duplicate participant", mutate: func(g *memberLookup) { g.allMembers[1] = g.allMembers[0] }, wantCalls: []string{"requester-id", "target-id"}},
		{name: "participant from another channel", mutate: func(g *memberLookup) {
			g.allMembers[1] = &model.ChannelMember{ChannelId: "other-channel", UserId: "target-id"}
		}, wantCalls: []string{"requester-id", "target-id"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			members := validMemberGetter()
			tt.mutate(members)
			response, appErr := (&Plugin{userGetter: validUserGetter(), channelGetter: validChannelGetter(), memberGetter: members}).ExecuteCommand(nil, &model.CommandArgs{
				Command: "/export-dm @other",
				UserId:  "requester-id",
			})
			if appErr != nil {
				t.Fatalf("ExecuteCommand returned an AppError: %v", appErr)
			}
			if response.Text != "Unable to authorize that direct-message conversation." {
				t.Errorf("response text = %q", response.Text)
			}
			if strings.Join(members.calls, ",") != strings.Join(tt.wantCalls, ",") {
				t.Errorf("GetChannelMember calls = %v, want %v", members.calls, tt.wantCalls)
			}
		})
	}
}

func TestExecuteCommandHandlesChannelLookupFailures(t *testing.T) {
	lookupError := model.NewAppError("test", "lookup failed", nil, "", 500)
	tests := []struct {
		name     string
		channels *recordingChannelGetter
		want     string
	}{
		{
			name:     "enumeration error",
			channels: &recordingChannelGetter{err: lookupError},
			want:     "Unable to inspect your direct-message conversations.",
		},
		{
			name:     "no channels",
			channels: &recordingChannelGetter{},
			want:     "No direct-message conversation with @other exists.",
		},
		{
			name: "only group channel",
			channels: &recordingChannelGetter{channels: []*model.Channel{{
				Name: model.GetDMNameFromIds("requester-id", "target-id"),
				Type: model.ChannelTypeGroup,
			}}},
			want: "No direct-message conversation with @other exists.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, appErr := (&Plugin{userGetter: validUserGetter(), channelGetter: tt.channels}).ExecuteCommand(nil, &model.CommandArgs{
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
