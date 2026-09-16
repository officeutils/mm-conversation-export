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
			response, appErr := (&Plugin{}).ExecuteCommand(nil, &model.CommandArgs{
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
			response, appErr := (&Plugin{}).ExecuteCommand(nil, tt.args)
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
