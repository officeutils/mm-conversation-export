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

// Plugin is the server-side DM export plugin.
type Plugin struct {
	plugin.MattermostPlugin

	commandRegistrar commandRegistrar
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

// ExecuteCommand validates an export request before any conversation data is
// looked up. Mattermost supplies UserId from the authenticated command request.
func (p *Plugin) ExecuteCommand(_ *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	if args == nil || args.UserId == "" {
		return commandError("Unable to export direct messages without an authenticated requester."), nil
	}

	username, err := parseCommandUsername(args.Command)
	if err != nil {
		return commandError("Usage: /export-dm @username"), nil
	}

	return &model.CommandResponse{
		ResponseType: "ephemeral",
		Text:         fmt.Sprintf("Preparing a direct-message export with @%s.", username),
	}, nil
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
