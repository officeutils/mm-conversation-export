package main

import "github.com/mattermost/mattermost/server/public/model"

// AttachmentMetadata is the subset of FileInfo that is safe and useful in an
// export. ID is an opaque identifier only; it must not be converted into a
// download URL.
type AttachmentMetadata struct {
	ID       string
	Filename string
	Size     int64
	MIMEType string
}

// collectAttachmentMetadata resolves only metadata for file IDs supplied by
// the already-authorized posts. It deliberately uses GetFileInfo rather than
// an API that reads attachment contents.
func collectAttachmentMetadata(files fileInfoGetter, posts []*model.Post) (map[string][]AttachmentMetadata, *model.AppError) {
	attachmentsByPost := make(map[string][]AttachmentMetadata)

	for _, post := range posts {
		if post == nil {
			continue
		}

		for _, fileID := range post.FileIds {
			info, appErr := files.GetFileInfo(fileID)
			if appErr != nil {
				return nil, appErr
			}
			if info == nil || info.Id != fileID {
				return nil, model.NewAppError(
					"collectAttachmentMetadata",
					"received invalid attachment metadata",
					nil,
					"",
					500,
				)
			}

			attachmentsByPost[post.Id] = append(attachmentsByPost[post.Id], AttachmentMetadata{
				ID:       info.Id,
				Filename: info.Name,
				Size:     info.Size,
				MIMEType: info.MimeType,
			})
		}
	}

	return attachmentsByPost, nil
}
