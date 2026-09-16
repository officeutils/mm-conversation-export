package main

import (
	"reflect"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
)

func TestCollectAttachmentMetadata(t *testing.T) {
	files := &recordingFileInfoGetter{
		infos: map[string]*model.FileInfo{
			"file-a": {Id: "file-a", Name: "notes.txt", Size: 42, MimeType: "text/plain"},
			"file-b": {Id: "file-b", Name: "photo.png", Size: 2048, MimeType: "image/png"},
		},
		errs: map[string]*model.AppError{},
	}
	posts := []*model.Post{
		{Id: "post-1", FileIds: []string{"file-a", "file-b"}},
		{Id: "post-2"},
	}

	got, appErr := collectAttachmentMetadata(files, posts)
	if appErr != nil {
		t.Fatalf("collectAttachmentMetadata returned an AppError: %v", appErr)
	}
	if !reflect.DeepEqual(files.calls, []string{"file-a", "file-b"}) {
		t.Fatalf("GetFileInfo calls = %#v", files.calls)
	}
	want := []AttachmentMetadata{
		{ID: "file-a", Filename: "notes.txt", Size: 42, MIMEType: "text/plain"},
		{ID: "file-b", Filename: "photo.png", Size: 2048, MIMEType: "image/png"},
	}
	if !reflect.DeepEqual(got["post-1"], want) {
		t.Errorf("metadata = %#v, want %#v", got["post-1"], want)
	}
	if _, ok := got["post-2"]; ok {
		t.Error("post without files unexpectedly has an attachment entry")
	}
}

func TestCollectAttachmentMetadataRejectsLookupFailures(t *testing.T) {
	lookupError := model.NewAppError("test", "lookup failed", nil, "", 500)
	for _, tt := range []struct {
		name  string
		files *recordingFileInfoGetter
	}{
		{
			name: "API error",
			files: &recordingFileInfoGetter{
				infos: map[string]*model.FileInfo{},
				errs:  map[string]*model.AppError{"file-id": lookupError},
			},
		},
		{
			name:  "nil metadata",
			files: &recordingFileInfoGetter{infos: map[string]*model.FileInfo{}, errs: map[string]*model.AppError{}},
		},
		{
			name: "mismatched identifier",
			files: &recordingFileInfoGetter{
				infos: map[string]*model.FileInfo{"file-id": {Id: "different-id"}},
				errs:  map[string]*model.AppError{},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, appErr := collectAttachmentMetadata(tt.files, []*model.Post{{Id: "post-id", FileIds: []string{"file-id"}}})
			if appErr == nil {
				t.Fatal("collectAttachmentMetadata returned no error")
			}
			if got != nil {
				t.Errorf("metadata = %#v, want nil", got)
			}
		})
	}
}

func TestExecuteCommandGetsFileInfoWithoutAttachmentContents(t *testing.T) {
	posts := &recordingPostGetter{postList: &model.PostList{
		Order: []string{"post-id"},
		Posts: map[string]*model.Post{"post-id": {Id: "post-id", FileIds: []string{"file-id"}}},
	}}
	files := &recordingFileInfoGetter{
		infos: map[string]*model.FileInfo{"file-id": {Id: "file-id", Name: "document.pdf", Size: 128, MimeType: "application/pdf"}},
		errs:  map[string]*model.AppError{},
	}

	response, appErr := (&Plugin{
		userGetter:    validUserGetter(),
		channelGetter: validChannelGetter(),
		memberGetter:  validMemberGetter(),
		postGetter:    posts,
		exportStore:   validExportStore(),
		fileGetter:    files,
	}).ExecuteCommand(nil, &model.CommandArgs{Command: "/export-dm @other", UserId: "requester-id"})
	if appErr != nil {
		t.Fatalf("ExecuteCommand returned an AppError: %v", appErr)
	}
	if response.Text != "[Download your direct-message export with @other](/plugins/com.github.officeutils.dm-export/download?token=test-token). This one-time link expires in 10 minutes." {
		t.Fatalf("response text = %q", response.Text)
	}
	if !reflect.DeepEqual(files.calls, []string{"file-id"}) {
		t.Errorf("GetFileInfo calls = %#v", files.calls)
	}
}
