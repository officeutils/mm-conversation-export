# Mattermost Conversation Export

Mattermost Conversation Export is a server-side plugin for Mattermost Community
Edition that lets signed-in users export one-to-one direct-message
conversations they participate in. Administrators may also opt in to current
public/private-channel export; it is disabled by default, and users can export
only a current channel they belong to and are authorized to read. Administrator
status does not override those checks, and callers cannot select an arbitrary
inaccessible channel. The plugin does not recover deleted messages or provide
compliance, organization-wide, or direct-database export.

## Compatibility

- **Minimum Mattermost version:** 10.11.2. The manifest enforces this minimum
  through `min_server_version`.
- The plugin uses the supported public Plugin API available in Mattermost
  10.11.2. The exact method signatures and the version-specific behavior on
  which it relies are recorded in
  [the 10.11.2 API contract audit](docs/mattermost-10.11.2-api-contracts.md).
- Release packages contain server executables for Linux (amd64 and arm64),
  macOS (amd64 and arm64), and Windows (amd64). There is no webapp component.
- This project does not claim compatibility with Mattermost releases older
  than 10.11.2. Run the test suite and validate the plugin in a staging system
  before deploying it with a newer server release.

## Build and package

Building requires Go 1.23 or later, GNU Make, and `tar`. From the repository
root:

```sh
make test       # run the Go test suite
make build      # cross-compile every executable declared in plugin.json
make package VERSION=0.1.0 # build dist/mm-conversation-export-0.1.0.tar.gz
```

`make package` creates a Mattermost plugin bundle with this layout:

```text
plugin.json
server/dist/plugin-linux-amd64
server/dist/plugin-linux-arm64
server/dist/plugin-darwin-amd64
server/dist/plugin-darwin-arm64
server/dist/plugin-windows-amd64.exe
```

Use `make clean` to remove generated `build/`, `dist/`, and `server/dist/`
directories. The package version and executable paths must continue to match
`plugin.json` when preparing a release.

## Installation

1. Build the bundle with `make package`, or download the equivalent archive
   from a trusted project release.
2. In Mattermost, open **System Console > Plugins > Plugin Management** and
   upload `dist/mm-conversation-export-0.1.0.tar.gz`.
3. Enable **Mattermost Conversation Export** on the same Plugin Management page.
4. Under **System Console > Plugins > Mattermost Conversation Export**, optionally set **Maximum
   Export Posts** to the maximum number of messages included in each export.
   The default is 1,000; accepted values are 1 through 10,000.
5. To permit current-channel exports, explicitly turn on **Enable channel
   export**. This opt-in is disabled by default.
6. Confirm that `/export-dm` and `/export-channel` appear in slash-command
   autocomplete. A registered `/export-channel` command still refuses exports
   while its setting is disabled.

Plugin uploads and custom plugins must be permitted by the Mattermost server's
plugin configuration. If the upload controls are unavailable, a Mattermost
system administrator must install the bundle using the deployment method
approved for that installation. In a multi-node deployment, see the storage
limitation below before enabling this plugin.

## Usage

### Direct messages by username

In any Mattermost channel, enter:

```text
/export-dm @username
```

The username is required (the leading `@` is optional), and no additional
arguments are accepted. The target must be another user with whom the requester
already has a one-to-one DM channel. Running the command does not create a DM.

On success, only the requester receives an ephemeral response containing a
download link. Open it while signed in as the same user. The link expires after
10 minutes and is consumed after a successful download. The downloaded file is
standalone HTML and contains:

- both participants and the export timestamp;
- at most the configured number of latest non-deleted posts (1,000 by default),
  retrieved across channel-history pages and ordered oldest to newest within
  that bounded result, including replies returned by the channel history API;
- each post's timestamp, author, text, and root/reply relationship; and
- attachment metadata (filename, size, MIME type, and opaque file ID).

The attachment files themselves are not included.

### Current channel

When **Enable channel export** has been turned on by a system administrator,
enter this with no arguments in the channel to export:

```text
/export-channel
```

The command exports only the current public, private, or one-to-one direct
channel identified by Mattermost's authenticated command context. It does not
accept a channel name or ID and cannot be invoked in one channel to export
another. The requester must be a current member and must have
`PermissionReadChannel`; public-channel visibility alone is not sufficient for
a guest who has not joined the channel. Direct channels retain the additional
two-participant validation used by `/export-dm`.

Current-channel exports use the same configured history limit, paginated latest
non-deleted history, reply inclusion, attachment-metadata handling, one-time
delivery, and process-local temporary storage as direct-message exports.

Only one active export may exist per user. Download or allow an existing link
to expire before requesting another. Each plugin process holds at most four
active exports, so a temporarily busy instance can reject a new request.

## Security boundaries

The export pipeline applies the following boundaries explicitly:

- **Authenticated requester only.** The slash command uses Mattermost's
  authenticated `CommandArgs.UserId`. Downloads use the
  `Mattermost-User-Id` header that Mattermost sets after authenticating the
  request through its plugin router. The HTTP handler must not be exposed on
  an independent server or behind a proxy that bypasses that router.
- **Requester-owned DM only.** The plugin enumerates the requester's existing
  channels, accepts only `ChannelTypeDirect`, and independently verifies that
  both the requester and target are the only two channel members before it
  reads posts. A user cannot supply a channel ID. Administrator status does
  not bypass these checks.
- **Authorized current channel only.** The optional `/export-channel` path
  accepts only the `CommandArgs.ChannelId` supplied for the current active
  public, private, or direct channel. It independently requires matching
  membership and `PermissionReadChannel` before reading posts. Administrators
  have no override: they must pass the same membership and permission checks.
- **Owner-bound delivery.** Download tokens contain 32 bytes of cryptographic
  randomness, expire after 10 minutes, are bound to the requesting user, and
  permit one successful download. Missing, invalid, expired, already claimed,
  and wrong-owner tokens do not disclose which check failed.
- **No direct storage access.** All users, channels, memberships, posts, and
  file metadata are read through the supported Mattermost Plugin API. The
  plugin does not query the Mattermost database or create channels.
- **Safe output.** User-controlled values are rendered with Go's
  context-aware `html/template` escaping. Downloads use an attachment content
  disposition, no-store cache headers, MIME sniffing protection, and a sandbox
  Content Security Policy.
- **Memory-only data.** Generated HTML and token metadata remain in plugin
  process memory until successful download or expiry. They are not written to
  disk or uploaded back into Mattermost by the plugin.

These controls restrict what the plugin releases; they cannot control copies
after a user downloads an export. Treat the resulting HTML file as sensitive
conversation data and store or share it according to your organization's
policies.

## Limitations and non-goals

- `/export-dm` supports only one-to-one DM channels. The disabled-by-default
  `/export-channel` opt-in additionally supports the current public or private
  channel. Group messages, archived channels, and self-DMs are rejected.
- An export contains up to the configured limit of the newest non-deleted posts
  (1,000 by default and at most 10,000), retrieved using paginated channel
  history. There is no deleted-post recovery, retention override, or legal-hold
  behavior.
- Mattermost 10.11.2 does not expose a reliable, generally applicable
  "since this requester joined" history boundary for this export path. An
  authorized current member can therefore export the configured latest-history
  window, including posts from before they joined if those posts remain
  visible through Mattermost's channel-history API.
- Attachment metadata is included, but attachment bytes, previews, and
  download URLs are not.
- Tokens and generated exports are process-local and memory-only. In a
  multi-node Mattermost deployment, a download routed to a different plugin
  instance will not find the token. Restarts also invalidate outstanding links.
  Use sticky routing to the same Mattermost node if evaluating this plugin in
  such a deployment; durable or shared storage is not implemented.
- The fixed capacity is four active exports per plugin instance and one per
  user. These limits and the 10-minute lifetime are not currently configurable.
- There is no scheduled, bulk, administrative, compliance, or Enterprise-only
  export path, and no webapp UI.
- The plugin relies on Mattermost 10.11.2 channel-history behavior for reply
  inclusion and non-deleted results. See the
  [API contract audit](docs/mattermost-10.11.2-api-contracts.md) for the audited
  assumptions.

## Development

Run the complete checks before submitting a change:

```sh
make test
make package
```

The server package tests cover command validation, DM authorization, bounded
post retrieval and ordering, reply inclusion, attachment metadata, HTML
escaping, temporary storage, and authenticated delivery.
