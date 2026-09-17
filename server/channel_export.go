package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

func isExportChannelCommand(args *model.CommandArgs) bool {
	if args == nil {
		return false
	}
	fields := strings.Fields(args.Command)
	return len(fields) > 0 && fields[0] == "/"+channelCommandTrigger
}

func (p *Plugin) executeExportChannelCommand(args *model.CommandArgs) *model.CommandResponse {
	if args.Command != "/"+channelCommandTrigger {
		return commandError("Usage: /export-channel")
	}
	if args.UserId == "" || args.ChannelId == "" {
		return commandError("Unable to export the current channel.")
	}

	channels := p.currentChannelGetter
	if channels == nil {
		channels = p.API
	}
	channel, appErr := channels.GetChannel(args.ChannelId)
	if appErr != nil || channel == nil || channel.Id != args.ChannelId || channel.DeleteAt != 0 ||
		(channel.Type != model.ChannelTypeOpen && channel.Type != model.ChannelTypePrivate) {
		return commandError("Unable to export the current channel.")
	}

	members := p.memberGetter
	if members == nil {
		members = p.API
	}
	member, appErr := members.GetChannelMember(channel.Id, args.UserId)
	if appErr != nil || !isExpectedMember(member, channel.Id, args.UserId) {
		return commandError("Unable to export the current channel.")
	}

	permissions := p.permissionChecker
	if permissions == nil {
		permissions = p.API
	}
	if !permissions.HasPermissionToChannel(args.UserId, channel.Id, model.PermissionReadChannel) {
		return commandError("Unable to export the current channel.")
	}

	postsAPI := p.postGetter
	if postsAPI == nil {
		postsAPI = p.API
	}
	maxPosts := p.maxExportPosts()
	posts, appErr := getSortedChannelPosts(postsAPI, channel.Id, maxPosts)
	if appErr != nil {
		return commandError("Unable to read the current channel.")
	}

	files := p.fileGetter
	if files == nil {
		files = p.API
	}
	attachments, appErr := collectAttachmentMetadata(files, posts)
	if appErr != nil {
		return commandError("Unable to read attachment metadata for the current channel.")
	}

	users := p.userGetter
	if users == nil {
		users = p.API
	}
	authors := resolvePostAuthors(users, posts)
	contents, err := renderChannelHTMLExport(channel, authors, maxPosts, posts, attachments)
	if err != nil {
		return commandError("Unable to render the channel export.")
	}
	if p.exportStore == nil {
		return commandError("Export delivery is temporarily unavailable.")
	}
	exportedAt := time.Now()
	if p.now != nil {
		exportedAt = p.now()
	}
	token, err := p.exportStore.Put(args.UserId, channelExportFilename(channel.Name, exportedAt), contents)
	if err != nil {
		return commandError("Unable to store the channel export. Please download any existing export or try again later.")
	}

	return &model.CommandResponse{ResponseType: "ephemeral", Text: fmt.Sprintf("[Download your channel export](/plugins/%s/download?token=%s). This one-time link expires in 10 minutes.", pluginID, token)}
}

func resolvePostAuthors(users userGetter, posts []*model.Post) map[string]string {
	authors := make(map[string]string)
	for _, post := range posts {
		if post == nil || post.UserId == "" {
			continue
		}
		if _, seen := authors[post.UserId]; seen {
			continue
		}
		authors[post.UserId] = "Unknown user"
		user, appErr := users.GetUser(post.UserId)
		if appErr == nil && user != nil && user.Id == post.UserId {
			authors[post.UserId] = exportUserName(user)
		}
	}
	return authors
}

var unsafeChannelFilenameCharacters = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func channelExportFilename(name string, exportedAt time.Time) string {
	name = strings.Trim(unsafeChannelFilenameCharacters.ReplaceAllString(name, "-"), ".-_")
	if name == "" {
		name = "channel"
	}
	return fmt.Sprintf("channel-%s-%s.html", name, exportedAt.UTC().Format("2006-01-02-150405"))
}
