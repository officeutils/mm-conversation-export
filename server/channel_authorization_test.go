package main

import (
	"strings"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
)

func TestExecuteCommandFindsExistingDirectChannel(t *testing.T) {
	channels := &recordingChannelGetter{channels: []*model.Channel{
		nil,
		{Id: "public", Name: model.GetDMNameFromIds("requester-id", "target-id"), Type: model.ChannelTypeOpen},
		{Id: "group", Name: model.GetDMNameFromIds("requester-id", "target-id"), Type: model.ChannelTypeGroup},
		{Id: "different-dm", Name: model.GetDMNameFromIds("requester-id", "someone-else"), Type: model.ChannelTypeDirect},
		{Id: "direct-channel-id", Name: model.GetDMNameFromIds("target-id", "requester-id"), Type: model.ChannelTypeDirect},
	}}

	response, appErr := (&Plugin{userGetter: validUserGetter(), channelGetter: channels, memberGetter: validMemberGetter(), postGetter: validPostGetter(), exportStore: validExportStore()}).ExecuteCommand(nil, &model.CommandArgs{
		Command: "/export-dm @other",
		UserId:  "requester-id",
	})
	if appErr != nil {
		t.Fatalf("ExecuteCommand returned an AppError: %v", appErr)
	}
	if response.Text != "[Download your direct-message export with @other](/plugins/com.github.officeutils.dm-export/download?token=test-token). This one-time link expires in 10 minutes." {
		t.Errorf("response text = %q", response.Text)
	}
}

func TestExecuteCommandVerifiesBothMembershipsAndParticipantSet(t *testing.T) {
	members := validMemberGetter()

	response, appErr := (&Plugin{
		userGetter:    validUserGetter(),
		channelGetter: validChannelGetter(),
		memberGetter:  members,
		postGetter:    validPostGetter(),
		exportStore:   validExportStore(),
	}).ExecuteCommand(nil, &model.CommandArgs{Command: "/export-dm @other", UserId: "requester-id"})
	if appErr != nil {
		t.Fatalf("ExecuteCommand returned an AppError: %v", appErr)
	}
	if response.Text != "[Download your direct-message export with @other](/plugins/com.github.officeutils.dm-export/download?token=test-token). This one-time link expires in 10 minutes." {
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
			g.allMembers = append(g.allMembers, model.ChannelMember{ChannelId: "direct-channel-id", UserId: "intruder-id"})
		}, wantCalls: []string{"requester-id", "target-id"}},
		{name: "duplicate participant", mutate: func(g *memberLookup) { g.allMembers[1] = g.allMembers[0] }, wantCalls: []string{"requester-id", "target-id"}},
		{name: "participant from another channel", mutate: func(g *memberLookup) {
			g.allMembers[1] = model.ChannelMember{ChannelId: "other-channel", UserId: "target-id"}
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
