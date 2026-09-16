package main

import (
	"bytes"
	"fmt"
	"html/template"
	"sort"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

type exportTemplateData struct {
	Title   string
	Threads []exportThread
	Notice  string
}

type exportThread struct {
	Root        *exportMessage
	Replies     []exportMessage
	MissingRoot bool
	createAt    int64
	sortID      string
}

type exportMessage struct {
	TimestampISO string
	Timestamp    string
	Author       string
	Text         string
	Attachments  []string
	createAt     int64
	id           string
}

var exportHTMLTemplate = template.Must(template.New("dm-export").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}}</title>
  <style>
    body { margin: 0; background: #f5f6f8; color: #202124; font: 16px/1.5 system-ui, sans-serif; }
    main { max-width: 54rem; margin: 0 auto; padding: 2rem 1rem; }
    h1 { font-size: 1.5rem; overflow-wrap: anywhere; }
    .threads, .replies { margin: 0; padding: 0; list-style: none; }
    .thread { margin: 1rem 0; padding: 1rem; border: 1px solid #d9dde3; border-radius: .5rem; background: white; }
    .message header { margin-bottom: .5rem; }
    .message time { color: #5f6368; font-size: .875rem; }
    .message-text { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; font: inherit; }
    .replies { margin: 1rem 0 0 1.5rem; padding-left: 1rem; border-left: 3px solid #d9dde3; }
    .reply + .reply { margin-top: 1rem; }
    .missing-root { margin: 0 0 1rem; color: #5f6368; font-style: italic; }
    .attachments { overflow-wrap: anywhere; }
  </style>
</head>
<body>
  <main>
    <h1>{{.Title}}</h1>
    <p>{{.Notice}}</p>
    <ol class="threads">
      {{range .Threads}}<li class="thread">
        {{if .MissingRoot}}<p class="missing-root">Earlier message is not included in this export</p>{{else}}{{with .Root}}{{template "message" .}}{{end}}{{end}}
        {{if .Replies}}<ol class="replies">{{range .Replies}}<li class="reply">{{template "message" .}}</li>{{end}}</ol>{{end}}
      </li>{{end}}
    </ol>
  </main>
</body>
</html>
{{define "message"}}<article class="message">
  <header><strong>{{.Author}}</strong> · <time datetime="{{.TimestampISO}}">{{.Timestamp}}</time></header>
  <pre class="message-text">{{.Text}}</pre>
  {{if .Attachments}}<ul class="attachments">{{range .Attachments}}<li>{{.}}</li>{{end}}</ul>{{end}}
</article>{{end}}`))

// renderHTMLExport renders a complete, standalone HTML document. html/template
// supplies context-aware escaping for participant, post, and attachment data.
func renderHTMLExport(requester, target *model.User, _ time.Time, maxPosts int, posts []*model.Post, attachmentsByPost map[string][]AttachmentMetadata) ([]byte, error) {
	authors := map[string]string{}
	participants := make([]string, 0, 2)
	for _, user := range []*model.User{requester, target} {
		name := exportUserName(user)
		participants = append(participants, name)
		if user != nil {
			authors[user.Id] = name
		}
	}

	messageFor := func(post *model.Post) exportMessage {
		author := authors[post.UserId]
		if author == "" {
			author = post.UserId
		}
		attachments := make([]string, 0, len(attachmentsByPost[post.Id]))
		for _, attachment := range attachmentsByPost[post.Id] {
			attachments = append(attachments, attachment.Filename)
		}
		return exportMessage{
			TimestampISO: time.UnixMilli(post.CreateAt).UTC().Format(time.RFC3339Nano),
			Timestamp:    formatExportTime(post.CreateAt),
			Author:       author,
			Text:         post.Message,
			Attachments:  attachments,
			createAt:     post.CreateAt,
			id:           post.Id,
		}
	}

	threadsByRoot := make(map[string]*exportThread)
	rootOrder := make([]string, 0, len(posts))
	for _, post := range posts {
		if post == nil || post.RootId != "" {
			continue
		}
		message := messageFor(post)
		threadsByRoot[post.Id] = &exportThread{Root: &message, createAt: post.CreateAt, sortID: post.Id}
		rootOrder = append(rootOrder, post.Id)
	}

	orphanOrder := make([]string, 0)
	for _, post := range posts {
		if post == nil || post.RootId == "" {
			continue
		}
		thread := threadsByRoot[post.RootId]
		if thread == nil {
			thread = &exportThread{MissingRoot: true, createAt: post.CreateAt, sortID: post.RootId}
			threadsByRoot[post.RootId] = thread
			orphanOrder = append(orphanOrder, post.RootId)
		} else if thread.MissingRoot && post.CreateAt < thread.createAt {
			// A truncated thread is positioned by its earliest included reply,
			// regardless of the order returned by the API.
			thread.createAt = post.CreateAt
		}
		thread.Replies = append(thread.Replies, messageFor(post))
	}

	threads := make([]exportThread, 0, len(rootOrder)+len(orphanOrder))
	for _, rootID := range append(rootOrder, orphanOrder...) {
		thread := threadsByRoot[rootID]
		sort.Slice(thread.Replies, func(i, j int) bool {
			if thread.Replies[i].createAt == thread.Replies[j].createAt {
				return thread.Replies[i].id < thread.Replies[j].id
			}
			return thread.Replies[i].createAt < thread.Replies[j].createAt
		})
		threads = append(threads, *thread)
	}
	sort.Slice(threads, func(i, j int) bool {
		if threads[i].createAt == threads[j].createAt {
			return threads[i].sortID < threads[j].sortID
		}
		return threads[i].createAt < threads[j].createAt
	})

	data := exportTemplateData{
		Title:   "Direct messages: " + participants[0] + " and " + participants[1],
		Threads: threads,
		Notice:  fmt.Sprintf("Exported %d messages. Configured limit: %d.", len(posts), maxPosts),
	}

	var output bytes.Buffer
	if err := exportHTMLTemplate.Execute(&output, data); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func exportUserName(user *model.User) string {
	if user == nil {
		return "Unknown participant"
	}
	if user.Username != "" {
		return "@" + user.Username
	}
	return user.Id
}

func formatExportTime(milliseconds int64) string {
	return time.UnixMilli(milliseconds).UTC().Format("January 2, 2006 at 15:04:05 UTC")
}
