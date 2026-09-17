package main

import "github.com/mattermost/mattermost/server/public/model"

type recordingUserGetter struct {
	requester         *model.User
	requesterErr      *model.AppError
	target            *model.User
	targetErr         *model.AppError
	requestedUserID   string
	requestedUsername string
}

type recordingChannelGetter struct {
	channels       []*model.Channel
	err            *model.AppError
	teamID         string
	userID         string
	includeDeleted bool
	calls          int
}

type memberLookup struct {
	members          map[string]*model.ChannelMember
	memberErrors     map[string]*model.AppError
	allMembers       model.ChannelMembers
	allErr           *model.AppError
	calls            []string
	memberChannelIDs []string
	channelID        string
	page             int
	perPage          int
}

type recordingPostGetter struct {
	postList  *model.PostList
	postLists []*model.PostList
	err       *model.AppError
	channelID string
	page      int
	perPage   int
	calls     int
	pages     []int
	perPages  []int
}

type recordingFileInfoGetter struct {
	infos map[string]*model.FileInfo
	errs  map[string]*model.AppError
	calls []string
}

type recordingChannelPermissionChecker struct {
	allowed    bool
	userID     string
	channelID  string
	permission *model.Permission
	calls      int
}

func (c *recordingChannelPermissionChecker) HasPermissionToChannel(userID, channelID string, permission *model.Permission) bool {
	c.calls++
	c.userID = userID
	c.channelID = channelID
	c.permission = permission
	return c.allowed
}

func (g *recordingFileInfoGetter) GetFileInfo(fileID string) (*model.FileInfo, *model.AppError) {
	g.calls = append(g.calls, fileID)
	return g.infos[fileID], g.errs[fileID]
}

func (g *recordingPostGetter) GetPostsForChannel(channelID string, page, perPage int) (*model.PostList, *model.AppError) {
	g.channelID = channelID
	g.page = page
	g.perPage = perPage
	g.calls++
	g.pages = append(g.pages, page)
	g.perPages = append(g.perPages, perPage)
	if len(g.postLists) > page {
		return g.postLists[page], g.err
	}
	return g.postList, g.err
}

func (g *memberLookup) GetChannelMember(channelID, userID string) (*model.ChannelMember, *model.AppError) {
	g.calls = append(g.calls, userID)
	g.memberChannelIDs = append(g.memberChannelIDs, channelID)
	return g.members[userID], g.memberErrors[userID]
}

func (g *memberLookup) GetChannelMembers(channelID string, page, perPage int) (model.ChannelMembers, *model.AppError) {
	g.channelID = channelID
	g.page = page
	g.perPage = perPage
	return g.allMembers, g.allErr
}

func (g *recordingChannelGetter) GetChannelsForTeamForUser(teamID, userID string, includeDeleted bool) ([]*model.Channel, *model.AppError) {
	g.teamID = teamID
	g.userID = userID
	g.includeDeleted = includeDeleted
	g.calls++
	return g.channels, g.err
}

func (g *recordingUserGetter) GetUser(userID string) (*model.User, *model.AppError) {
	g.requestedUserID = userID
	return g.requester, g.requesterErr
}

func (g *recordingUserGetter) GetUserByUsername(username string) (*model.User, *model.AppError) {
	g.requestedUsername = username
	return g.target, g.targetErr
}

func validUserGetter() *recordingUserGetter {
	return &recordingUserGetter{
		requester: &model.User{Id: "requester-id", Username: "requester"},
		target:    &model.User{Id: "target-id", Username: "other"},
	}
}

func validChannelGetter() *recordingChannelGetter {
	return &recordingChannelGetter{channels: []*model.Channel{{
		Id:   "direct-channel-id",
		Name: model.GetDMNameFromIds("requester-id", "target-id"),
		Type: model.ChannelTypeDirect,
	}}}
}

func validMemberGetter() *memberLookup {
	requester := &model.ChannelMember{ChannelId: "direct-channel-id", UserId: "requester-id"}
	target := &model.ChannelMember{ChannelId: "direct-channel-id", UserId: "target-id"}
	return &memberLookup{
		members: map[string]*model.ChannelMember{
			"requester-id": requester,
			"target-id":    target,
		},
		memberErrors: map[string]*model.AppError{},
		allMembers:   model.ChannelMembers{*requester, *target},
	}
}

func validPostGetter() *recordingPostGetter {
	return &recordingPostGetter{postList: model.NewPostList()}
}

type recordingExportStore struct {
	ownerID  string
	filename string
	contents []byte
	putErr   error
}

func (s *recordingExportStore) Put(ownerID, filename string, contents []byte) (string, error) {
	s.ownerID = ownerID
	s.filename = filename
	s.contents = append([]byte(nil), contents...)
	if s.putErr != nil {
		return "", s.putErr
	}
	return "test-token", nil
}

func (*recordingExportStore) Claim(string, string) (storedExport, error) {
	return storedExport{}, errExportNotFound
}
func (*recordingExportStore) Finish(string, string, bool) {}

func validExportStore() *recordingExportStore { return &recordingExportStore{} }
