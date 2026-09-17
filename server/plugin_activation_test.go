package main

import (
	"errors"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
)

type recordingRegistrar struct {
	commands []*model.Command
	err      error
	failAt   int
}

func (r *recordingRegistrar) RegisterCommand(command *model.Command) error {
	r.commands = append(r.commands, command)
	if r.failAt == len(r.commands) {
		return r.err
	}
	return nil
}

func TestOnActivateRegistersExportDMCommand(t *testing.T) {
	registrar := &recordingRegistrar{}
	p := &Plugin{commandRegistrar: registrar}

	if err := p.OnActivate(); err != nil {
		t.Fatalf("OnActivate returned an error: %v", err)
	}

	if len(registrar.commands) != 2 {
		t.Fatalf("registered %d commands, want 2", len(registrar.commands))
	}
	command := registrar.commands[0]
	if command.Trigger != commandTrigger {
		t.Errorf("command trigger = %q, want %q", command.Trigger, commandTrigger)
	}
	if !command.AutoComplete {
		t.Error("command autocomplete is disabled")
	}
	if command.AutoCompleteHint != "@username" {
		t.Errorf("command autocomplete hint = %q, want %q", command.AutoCompleteHint, "@username")
	}

	channelCommand := registrar.commands[1]
	if channelCommand.Trigger != channelCommandTrigger {
		t.Errorf("channel command trigger = %q, want %q", channelCommand.Trigger, channelCommandTrigger)
	}
	if !channelCommand.AutoComplete {
		t.Error("channel command autocomplete is disabled")
	}
	if channelCommand.AutoCompleteHint != "" {
		t.Errorf("channel command autocomplete hint = %q, want empty", channelCommand.AutoCompleteHint)
	}
}

func TestOnActivateReturnsRegistrationError(t *testing.T) {
	for _, failAt := range []int{1, 2} {
		want := errors.New("registration failed")
		p := &Plugin{commandRegistrar: &recordingRegistrar{err: want, failAt: failAt}}

		if got := p.OnActivate(); !errors.Is(got, want) {
			t.Fatalf("registration %d: OnActivate error = %v, want %v", failAt, got, want)
		}
	}
}
