# Mattermost DM Export

Mattermost DM Export is a server-side plugin for Mattermost Community Edition
that lets a signed-in user export an existing one-to-one direct-message
conversation in a standalone HTML file. It is intentionally a narrow,
self-service feature: it is not an administrative, compliance, or database
export tool.

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
make package    # build and create dist/com.github.officeutils.dm-export-0.1.0.tar.gz
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
   upload `dist/com.github.officeutils.dm-export-0.1.0.tar.gz`.
3. Enable **DM Export** on the same Plugin Management page.
4. Confirm that `/export-dm` appears in the slash-command autocomplete list.

Plugin uploads and custom plugins must be permitted by the Mattermost server's
plugin configuration. If the upload controls are unavailable, a Mattermost
system administrator must install the bundle using the deployment method
approved for that installation. In a multi-node deployment, see the storage
limitation below before enabling this plugin.

## Usage

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
- at most the latest 100 non-deleted posts, ordered oldest to newest within
  that bounded result, including replies returned by the channel history API;
- each post's timestamp, author, text, and root/reply relationship; and
- attachment metadata (filename, size, MIME type, and opaque file ID).

The attachment files themselves are not included.

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

- Only one-to-one DM channels are supported. Group messages, public channels,
  private channels, and self-DMs are rejected.
- An export contains only the newest page of up to 100 non-deleted posts. There
  is no pagination, deleted-post recovery, retention override, or legal-hold
  behavior.
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
