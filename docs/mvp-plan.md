# MVP Implementation Plan

This document is the frozen implementation specification for the Mattermost Conversation Export MVP.

## Architecture

Build a server-side-only Go plugin for Mattermost 10.11.2+. The `/export-dm @username` command:

1. Parses and validates the target username.
2. Resolves the requester and target users.
3. Locates their existing DM through read-only channel enumeration.
4. Verifies the channel type and both memberships.
5. Retrieves at most the latest 100 non-deleted posts.
6. Collects attachment metadata.
7. Renders escaped standalone HTML.
8. Stores the export temporarily in memory.
9. Returns an ephemeral, requester-bound, one-time download link.

There is no webapp, direct database access, persistent archive, administrative path, arbitrary channel export, or channel-creation side effect.

## Mattermost 10.11.2+ API/interface assumptions

Use these supported public plugin interfaces:

- `plugin.Plugin.OnActivate() error`
- `plugin.Plugin.ExecuteCommand(*plugin.Context, *model.CommandArgs) (*model.CommandResponse, *model.AppError)`
- `plugin.Plugin.ServeHTTP(*plugin.Context, http.ResponseWriter, *http.Request)`
- `API.RegisterCommand(*model.Command) error`
- `API.GetUser(userID string)`
- `API.GetUserByUsername(username string)`
- `API.GetChannelsForTeamForUser(teamID, userID string, includeDeleted bool)`
- `API.GetChannelMember(channelID, userID string)`
- `API.GetChannelMembers(channelID string, page, perPage int)`, when needed to confirm the complete participant set
- `API.GetPostsForChannel(channelID string, page, perPage int)`
- `API.GetFileInfo(fileID string)`

Relevant models and constants include `model.Command`, `model.CommandArgs`, `model.CommandResponse`, `model.Channel`, `model.ChannelMember`, `model.Post`, `model.PostList`, `model.FileInfo`, and `model.ChannelTypeDirect`.

The exact signatures and behaviors are recorded in [the Mattermost 10.11.2 source audit](mattermost-10.11.2-api-contracts.md). `GetDirectChannel` and `GetPostThread` are not used in the MVP export path.

## Security flow

1. Require a non-empty `CommandArgs.UserId`; it is the authoritative requester identity.
2. Parse exactly one username with an optional leading `@`.
3. Resolve the requester with `GetUser` and the target with `GetUserByUsername`.
4. Reject identical requester and target IDs.
5. Call `GetChannelsForTeamForUser("", requesterID, false)`; the empty team ID returns non-deleted DM and group-message channels for that user.
6. Consider only channels with `Type == model.ChannelTypeDirect`; enumeration is not itself a DM-type or membership authorization check.
7. Identify the existing channel containing the target.
8. Independently require requester and target membership with `GetChannelMember`.
9. Use `GetChannelMembers` if necessary to confirm only the expected two participants.
10. Read posts only after every authorization check succeeds.

Never authorize from username text, administrator status, channel name, a caller-supplied channel ID, query parameters, or request-body identity. There is no administrator bypass, and the user never supplies a channel ID.

## Post retrieval behavior

Define `100` as a named limit constant and call exactly:

```go
GetPostsForChannel(channelID, 0, 100)
```

Do not paginate further. This is the newest page, returned newest-first through `PostList.Order`; do not infer order by iterating the `Posts` map. Mattermost excludes deleted posts by default, so do not request, recover, backfill, or reconstruct deleted content. Export every returned post; if fewer than 100 are available, export all of them. Sort chronologically by `(CreateAt, ID)`.

Thread replies are expected to be normal channel posts. Do not call `GetPostThread`. Maintain a Mattermost 10.11.2 regression/integration test proving that a reply appears in the channel result.

## HTML export behavior

Generate minimal standalone UTF-8 HTML containing:

- both participants;
- export timestamp;
- notice that at most the latest 100 non-deleted messages are included;
- message timestamp, author, and text;
- root/reply relationship where available; and
- attachment filename, byte size, MIME type, and safe file identifier where available.

Escape every user-controlled value with escaping-aware templates. Do not preserve arbitrary message HTML. Do not download or embed attachments, generate authorization-bypassing URLs, or re-upload the export as a Mattermost attachment.

## Temporary delivery model

Store export bytes and token metadata in memory behind a small interface. Tokens must be cryptographically random, short-lived, single-use, and bound to the requester ID. Enforce one active export per user and four active exports per plugin instance.

For `ServeHTTP`:

1. Obtain the Mattermost-authenticated identity from the server-supplied `Mattermost-User-Id` and reject the request when it is absent. Mattermost strips a client-supplied value before setting the header from the authenticated session.
2. Require a valid, unexpired token owned by that authenticated user.
3. Serve HTML with safe content type, attachment disposition, and no-store caching headers; `ServeHTTP` has no return value, so write errors and status codes directly.
4. Atomically invalidate the token and remove export data after successful download.
5. Remove expired entries opportunistically without scheduled cleanup.

A token alone is never sufficient authorization. Identity must not come from query parameters, request bodies, or untrusted client-provided headers. Verify the server's header handling against Mattermost 10.11.2 source during implementation.

## Repository structure

```text
plugin.json
go.mod
go.sum
Makefile
README.md
server/
  plugin.go
  command.go
  authorization.go
  conversation.go
  attachments.go
  renderer.go
  delivery.go
  api.go
  *_test.go
build/
```

`server/api.go` contains narrow mockable interfaces for the required Plugin API methods and temporary export storage.

## Implementation stages

Each stage must remain independently reviewable and avoid unrelated changes.

1. **Verify Mattermost 10.11.2 API contracts.** Confirm exact public method signatures, channel enumeration behavior, post-query semantics, file metadata fields, `ServeHTTP`, and trusted `Mattermost-User-Id` handling.
2. **Create the server-side plugin skeleton.** Add the Go module, manifest, server entry point, activation hook, `/export-dm` registration, and basic registration test.
3. **Implement command parsing and validation.** Accept exactly one username with an optional leading `@`; reject missing, malformed, or additional arguments and missing requester identity.
4. **Resolve requester and target users.** Resolve the requester from `CommandArgs.UserId`, resolve the target by username, handle lookup failures, and reject equal user IDs.
5. **Implement read-only existing-DM lookup.** Enumerate requester channels, retain direct channels, and identify the existing DM containing the target without calling `GetDirectChannel`.
6. **Authorize both DM participants.** Verify both memberships independently and, where needed, confirm the expected two-person participant set. Add focused security tests.
7. **Retrieve and sort the latest 100 posts.** Call `GetPostsForChannel(channelID, 0, 100)` once and sort all returned posts by `(CreateAt, ID)`.
8. **Verify thread replies in channel results.** Add a Mattermost 10.11.2 regression/integration test proving that a thread reply appears in the bounded channel result.
9. **Collect attachment metadata.** Resolve filename, size, MIME type, and safe identifier with `GetFileInfo` without downloading attachment contents.
10. **Render escaped standalone HTML.** Render all required export fields and the latest-100 notice using escaping-aware templates.
11. **Implement temporary export storage.** Add bounded in-memory storage, random owner-bound tokens, expiry, atomic consumption, and configured concurrency limits.
12. **Implement authenticated one-time download.** Add `ServeHTTP` authentication, ownership validation, safe response headers, expiry handling, and replay prevention.
13. **Wire the complete export pipeline.** Connect command handling, authorization, retrieval, metadata collection, rendering, storage, and ephemeral download-link delivery.
14. **Add negative and error-path coverage.** Test malformed input, missing users or DMs, self-DM requests, authorization failures, API errors, concurrency rejection, token mismatch, expiry, and replay.
15. **Add documentation and release setup.** Document installation, usage, security boundaries, limitations, build/package steps, and Mattermost 10.11.2 compatibility.

## Known limitations

- Minimum Mattermost version is 10.11.2.
- Exports contain at most the latest 100 non-deleted posts.
- Thread inclusion is protected by a regression/integration test.
- The HTTP identity header is trusted only on requests delivered through Mattermost's plugin router; the handler must not be independently mounted.
- In-memory export/token storage supports single-node deployments only.
- Self-DM export is disabled.
- Exports are temporary and never persisted.
- Group messages, public/private channels, administrative/compliance export, retention/legal hold, scheduled exports, and attachment contents are out of scope.
