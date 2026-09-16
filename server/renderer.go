package main

import (
	"bytes"
	"html/template"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

type exportTemplateData struct {
	Participants []string
	ExportedAt   string
	Notice       string
	Messages     []exportMessage
}

type exportMessage struct {
	ID          string
	Timestamp   string
	Author      string
	Text        string
	RootID      string
	Attachments []AttachmentMetadata
}

var exportHTMLTemplate = template.Must(template.New("dm-export").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Direct-message export</title>
</head>
<body>
  <main>
    <h1>Direct-message export</h1>
    <p><strong>Participants:</strong> {{range $index, $participant := .Participants}}{{if $index}}, {{end}}{{$participant}}{{end}}</p>
    <p><strong>Exported:</strong> <time datetime="{{.ExportedAt}}">{{.ExportedAt}}</time></p>
    <p><strong>Scope:</strong> {{.Notice}}</p>
    <ol>
      {{range .Messages}}<li data-post-id="{{.ID}}">
        <article>
          <header><time datetime="{{.Timestamp}}">{{.Timestamp}}</time> — <strong>{{.Author}}</strong></header>
          {{if .RootID}}<p>Reply to post <code>{{.RootID}}</code></p>{{else}}<p>Root post</p>{{end}}
          <pre>{{.Text}}</pre>
          {{if .Attachments}}<h2>Attachments</h2>
          <ul>{{range .Attachments}}
            <li><strong>{{.Filename}}</strong> — {{.Size}} bytes; MIME type: <code>{{.MIMEType}}</code>; file ID: <code>{{.ID}}</code></li>{{end}}
          </ul>{{end}}
        </article>
      </li>{{end}}
    </ol>
  </main>
</body>
</html>
`))

// renderHTMLExport renders a complete, standalone HTML document. html/template
// supplies context-aware escaping for participant, post, and attachment data.
func renderHTMLExport(requester, target *model.User, exportedAt time.Time, posts []*model.Post, attachmentsByPost map[string][]AttachmentMetadata) ([]byte, error) {
	authors := map[string]string{}
	participants := make([]string, 0, 2)
	for _, user := range []*model.User{requester, target} {
		name := exportUserName(user)
		participants = append(participants, name)
		if user != nil {
			authors[user.Id] = name
		}
	}

	messages := make([]exportMessage, 0, len(posts))
	for _, post := range posts {
		if post == nil {
			continue
		}

		author := authors[post.UserId]
		if author == "" {
			author = post.UserId
		}
		messages = append(messages, exportMessage{
			ID:          post.Id,
			Timestamp:   formatExportTime(post.CreateAt),
			Author:      author,
			Text:        post.Message,
			RootID:      post.RootId,
			Attachments: attachmentsByPost[post.Id],
		})
	}

	data := exportTemplateData{
		Participants: participants,
		ExportedAt:   exportedAt.UTC().Format(time.RFC3339),
		Notice:       "At most the latest 100 non-deleted messages are included.",
		Messages:     messages,
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
	return time.UnixMilli(milliseconds).UTC().Format(time.RFC3339Nano)
}
