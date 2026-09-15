package main

import (
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

const commandTrigger = "export-dm"

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

func main() {
	plugin.ClientMain(&Plugin{})
}
