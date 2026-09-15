# Mattermost 10.11.2 API contracts

This note records the source audit for the plugin's minimum supported Mattermost
release. Links intentionally point at the `v10.11.2` tag rather than the default
branch. The findings below are the contracts the MVP may rely on; behavior not
listed here should be treated as unverified.

## Public plugin method signatures

The public hooks used by the MVP are:

```go
OnActivate() error
ExecuteCommand(c *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError)
ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request)
```

The public `plugin.API` methods have these signatures:

```go
RegisterCommand(command *model.Command) error
GetUser(userID string) (*model.User, *model.AppError)
GetUserByUsername(name string) (*model.User, *model.AppError)
GetChannelsForTeamForUser(teamID, userID string, includeDeleted bool) ([]*model.Channel, *model.AppError)
GetChannelMember(channelID, userID string) (*model.ChannelMember, *model.AppError)
GetChannelMembers(channelID string, page, perPage int) (model.ChannelMembers, *model.AppError)
GetPostsForChannel(channelID string, page, perPage int) (*model.PostList, *model.AppError)
GetFileInfo(fileID string) (*model.FileInfo, *model.AppError)
```

Sources: [`server/public/plugin/hooks.go`](https://github.com/mattermost/mattermost/blob/v10.11.2/server/public/plugin/hooks.go),
[`server/public/plugin/api.go`](https://github.com/mattermost/mattermost/blob/v10.11.2/server/public/plugin/api.go), and
[`server/plugin/api.go`](https://github.com/mattermost/mattermost/blob/v10.11.2/server/plugin/api.go).

Two details matter when defining narrow mock interfaces: `GetChannelMembers`
returns the named slice type `model.ChannelMembers`, not `[]*model.ChannelMember`,
and every read method above returns a `*model.AppError` rather than a Go `error`.

## Channel enumeration

`GetChannelsForTeamForUser` delegates to the application channel lookup. The
lookup first obtains the channels visible to the specified user and then retains
channels whose `TeamId` equals `teamID`. Direct and group-message channels have
an empty `TeamId`. Consequently:

```go
GetChannelsForTeamForUser("", requesterID, false)
```

returns the requester's non-deleted direct **and** group-message channels. It is
not a DM-only operation. The caller must still require
`channel.Type == model.ChannelTypeDirect`; `includeDeleted == false` only controls
deleted channels and is not an authorization check.

This lookup is read-only. In contrast, `GetDirectChannel` is deliberately not
used because its application path can create the direct channel when none
exists.

Sources: [`server/channels.go`](https://github.com/mattermost/mattermost/blob/v10.11.2/server/channels.go),
[`server/plugin/api.go`](https://github.com/mattermost/mattermost/blob/v10.11.2/server/plugin/api.go), and
[`server/public/model/channel.go`](https://github.com/mattermost/mattermost/blob/v10.11.2/server/public/model/channel.go).

## Post-query semantics

`GetPostsForChannel(channelID, page, perPage)` uses the ordinary channel-post
page query. With `(page, perPage) == (0, 100)`, the bound is applied to the newest
page and the returned `PostList.Order` is newest-first. `PostList.Posts` is a map,
so map iteration must never be used to infer display order. The exporter will
collect the posts named by `Order` and then sort them by `(CreateAt, Id)` for its
oldest-first output.

The default query does not request deleted posts. It also uses the uncollapsed
channel history query, so replies are ordinary channel posts in this result; no
`GetPostThread` call is required. The reply behavior remains worth protecting
with the planned 10.11.2 integration/regression test because collapsed-thread
REST options are a separate query mode.

Sources: [`server/plugin/api.go`](https://github.com/mattermost/mattermost/blob/v10.11.2/server/plugin/api.go),
[`server/post.go`](https://github.com/mattermost/mattermost/blob/v10.11.2/server/post.go), and
[`server/public/model/post.go`](https://github.com/mattermost/mattermost/blob/v10.11.2/server/public/model/post.go).

## Attachment metadata

`GetFileInfo(fileID)` returns `*model.FileInfo`. The export fields map directly
to these public fields:

| Export value | `model.FileInfo` field | Go type |
| --- | --- | --- |
| Safe opaque identifier | `Id` | `string` |
| Filename | `Name` | `string` |
| Byte size | `Size` | `int64` |
| MIME type | `MimeType` | `string` |

The post supplies attachment identifiers in `Post.FileIds []string`. File
metadata is not file content, and a file ID must not be turned into a public or
authorization-bypassing download URL.

Sources: [`server/public/model/file_info.go`](https://github.com/mattermost/mattermost/blob/v10.11.2/server/public/model/file_info.go),
[`server/public/model/post.go`](https://github.com/mattermost/mattermost/blob/v10.11.2/server/public/model/post.go), and
[`server/plugin/api.go`](https://github.com/mattermost/mattermost/blob/v10.11.2/server/plugin/api.go).

## `ServeHTTP` and authenticated identity

Mattermost mounts a plugin's `ServeHTTP` hook below the plugin route and calls
it with the plugin context, response writer, and request. The hook has no return
value; the implementation must write the status and response itself.

Before the request reaches the plugin, the Mattermost server removes any
inbound `Mattermost-User-Id` value. After authenticating the request through the
normal Mattermost session handling, it sets that header to the authenticated
user ID. It is absent for an anonymous request. Thus the handler must use:

```go
requesterID := r.Header.Get("Mattermost-User-Id")
```

and reject an empty value. It must not accept identity from a query parameter,
request body, cookie parsed by plugin code, or alternate header. This trust
statement applies only to requests that reached `ServeHTTP` through the
Mattermost plugin router; it would not apply if the handler were mounted on an
independent HTTP server.

The identity header authenticates the requester, but does not authorize an
export by itself. The download path must additionally consume a live token whose
stored owner ID exactly matches the header value.

Sources: [`server/plugin/api.go`](https://github.com/mattermost/mattermost/blob/v10.11.2/server/plugin/api.go),
[`server/api4/plugin.go`](https://github.com/mattermost/mattermost/blob/v10.11.2/server/api4/plugin.go), and
[`server/public/model/client4.go`](https://github.com/mattermost/mattermost/blob/v10.11.2/server/public/model/client4.go).

## Implementation consequences

1. Enumerate with an empty team ID, then explicitly filter to direct channels.
2. Verify requester and target membership independently before reading posts.
3. Call `GetPostsForChannel(channelID, 0, 100)` once, respect the returned IDs,
   and impose deterministic chronological output ordering.
4. Fetch only `FileInfo`; never fetch attachment bytes.
5. Reject an empty `Mattermost-User-Id`, then require exact token ownership.
6. Keep a 10.11.2 integration test for reply inclusion even though the audited
   default query is uncollapsed.
