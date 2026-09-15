# AGENTS.md

## Project

Open-source Mattermost CE plugin for self-service export of a user's own direct messages.

## Core constraints

- Server-side Go plugin first.
- No direct database access.
- No Enterprise code.
- No administrative export.
- Current user may export only DMs they participate in.
- DM channels only.
- Prefer supported Mattermost Plugin API.
- Security checks must be explicit and testable.
- Keep tasks small and commits reviewable.

## Development

- Run tests after changes.
- Do not expand scope without discussion.
- Flag uncertain Mattermost API behavior.
- Prefer simple code over abstractions unless justified.
