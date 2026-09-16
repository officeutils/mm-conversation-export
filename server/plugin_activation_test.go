package main

import (
	"errors"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
)

type recordingRegistrar struct {
	command *model.Command
	err     error
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
