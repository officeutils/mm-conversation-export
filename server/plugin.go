package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

const commandTrigger = "export-dm"

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

type commandRegistrar interface {
	RegisterCommand(command *model.Command) error
}

type userGetter interface {
	GetUser(userID string) (*model.User, *model.AppError)
	GetUserByUsername(username string) (*model.User, *model.AppError)
}

type channelGetter interface {
	GetChannelsForTeamForUser(teamID, userID string, includeDeleted bool) ([]*model.Channel, *model.AppError)
}

type channelMemberGetter interface {
	GetChannelMember(channelID, userID string) (*model.ChannelMember, *model.AppError)
	GetChannelMembers(channelID string, page, perPage int) (model.ChannelMembers, *model.AppError)
}

// Plugin is the server-side DM export plugin.
type Plugin struct {
	plugin.MattermostPlugin

	commandRegistrar commandRegistrar
	userGetter       userGetter
	channelGetter    channelGetter
	memberGetter     channelMemberGetter
}

// OnActivate registers the slash command exposed by the plugin.
func (p *Plugin) OnActivate() error {
	registrar := p.commandRegistrar
	if registrar == nil {
		registrar = p.API
	}

	return registrar.RegisterCommand(&model.Command{
		Trigger:          commandTrigger,
		AutoComplete:     true,
		AutoCompleteDesc: "Export a direct-message conversation",
		AutoCompleteHint: "@username",
	})
}

// ExecuteCommand validates an export request and locates its existing direct
// channel. Mattermost supplies UserId from the authenticated command request.
func (p *Plugin) ExecuteCommand(_ *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	if args == nil || args.UserId == "" {
		return commandError("Unable to export direct messages without an authenticated requester."), nil
	}

	username, err := parseCommandUsername(args.Command)
	if err != nil {
		return commandError("Usage: /export-dm @username"), nil
	}

	users := p.userGetter
	if users == nil {
		users = p.API
	}

	requester, appErr := users.GetUser(args.UserId)
	if appErr != nil || requester == nil {
		return commandError("Unable to resolve the authenticated requester."), nil
	}

	target, appErr := users.GetUserByUsername(username)
	if appErr != nil || target == nil {
		return commandError(fmt.Sprintf("Unable to find user @%s.", username)), nil
	}

	if requester.Id == target.Id {
		return commandError("You cannot export a direct-message conversation with yourself."), nil
	}

	channels := p.channelGetter
	if channels == nil {
		channels = p.API
	}

	requesterChannels, appErr := channels.GetChannelsForTeamForUser("", requester.Id, false)
	if appErr != nil {
		return commandError("Unable to inspect your direct-message conversations."), nil
	}

	directChannel := findDirectChannel(requesterChannels, requester.Id, target.Id)
	if directChannel == nil {
		return commandError(fmt.Sprintf("No direct-message conversation with @%s exists.", username)), nil
	}

	members := p.memberGetter
	if members == nil {
		members = p.API
	}

	if !authorizeDirectChannel(members, directChannel.Id, requester.Id, target.Id) {
		return commandError("Unable to authorize that direct-message conversation."), nil
	}

	return &model.CommandResponse{
		ResponseType: "ephemeral",
		Text:         fmt.Sprintf("Preparing a direct-message export with @%s.", username),
	}, nil
}

func authorizeDirectChannel(members channelMemberGetter, channelID, requesterID, targetID string) bool {
	requesterMember, appErr := members.GetChannelMember(channelID, requesterID)
	if appErr != nil || !isExpectedMember(requesterMember, channelID, requesterID) {
		return false
	}

	targetMember, appErr := members.GetChannelMember(channelID, targetID)
	if appErr != nil || !isExpectedMember(targetMember, channelID, targetID) {
		return false
	}

	// Fetch at most three members: a third result is enough to reject a channel
	// that is not the expected two-person conversation.
	channelMembers, appErr := members.GetChannelMembers(channelID, 0, 3)
	if appErr != nil || len(channelMembers) != 2 {
		return false
	}

	seen := map[string]bool{}
	for _, member := range channelMembers {
		if member.ChannelId != channelID ||
			(member.UserId != requesterID && member.UserId != targetID) || seen[member.UserId] {
			return false
		}
		seen[member.UserId] = true
	}

	return seen[requesterID] && seen[targetID]
}

func isExpectedMember(member *model.ChannelMember, channelID, userID string) bool {
	return member != nil && member.ChannelId == channelID && member.UserId == userID
}

func findDirectChannel(channels []*model.Channel, requesterID, targetID string) *model.Channel {
	directChannelName := model.GetDMNameFromIds(requesterID, targetID)
	for _, channel := range channels {
		if channel != nil && channel.Type == model.ChannelTypeDirect && channel.Name == directChannelName {
			return channel
		}
	}

	return nil
}

func parseCommandUsername(command string) (string, error) {
	fields := strings.Fields(command)
	if len(fields) != 2 || fields[0] != "/"+commandTrigger {
		return "", fmt.Errorf("expected /%s followed by one username", commandTrigger)
	}

	username := strings.TrimPrefix(fields[1], "@")
	if !usernamePattern.MatchString(username) {
		return "", fmt.Errorf("invalid username")
	}

	return username, nil
}

func commandError(message string) *model.CommandResponse {
	return &model.CommandResponse{
		ResponseType: "ephemeral",
		Text:         message,
	}
}

func main() {
	plugin.ClientMain(&Plugin{})
}
