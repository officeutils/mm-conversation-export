# AGENTS.md

## Project

Open-source Mattermost CE plugin for self-service export of a user's own direct
messages, with an administrator-enabled current-channel export for public and
private channels.

## Core constraints

- Server-side Go plugin first.
- No Enterprise code.
- DM export remains restricted to direct-message channels where the requester
  is a participant.
- Channel export is available only when the server administrator explicitly
  enables `EnableChannelExport`.
- Channel export may operate only on the current public or private channel.
  The requester must be a current member and have `PermissionReadChannel`.
- Administrator status does not override channel membership or
  `PermissionReadChannel` checks.
- Do not accept arbitrary caller-supplied channel IDs for channel export.
- Group-message export and self-DM export are unsupported.
- Deleted messages, inaccessible channels, hidden-channel bypass, compliance
  export, organization-wide export, and direct database access are out of
  scope.
- Prefer supported Mattermost Plugin API.
- Security checks must be explicit and testable.
- Keep tasks small and commits reviewable.

## Development

- Run tests after changes.
- Do not expand scope without discussion.
- Flag uncertain Mattermost API behavior.
- Prefer simple code over abstractions unless justified.
