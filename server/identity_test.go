package main

import (
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/svelle/mattermost-plugin-moderation/server/store/kvstore"
)

const bannedUserID = "abcdefghijklmnopqrstuvwxyz"

func TestBanHidesIdentityAndUnbanRestoresIt(t *testing.T) {
	p, api, kv := newTestPlugin(t, map[string]bool{bannedUserID: false})
	p.configuration = &configuration{HideBannedIdentity: true}

	original := &model.User{Id: bannedUserID, Username: "slur_name", FirstName: "Real", LastName: "Name", Nickname: "Nick"}
	current := original.DeepCopy()
	api.On("GetUser", bannedUserID).Return(func(string) *model.User { return current.DeepCopy() }, nil)
	api.On("UpdateUser", mock.Anything).Return(func(u *model.User) *model.User {
		current = u.DeepCopy()
		return current
	}, nil)
	api.On("UpdateUserActive", bannedUserID, mock.Anything).Return(nil)
	api.On("KVSetWithOptions", mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Maybe()
	api.On("KVDelete", mock.Anything).Return(nil).Maybe()
	api.On("PublishWebSocketEvent", mock.Anything, mock.Anything, mock.Anything).Maybe()

	assert.NoError(t, p.banUser(sysAdminID, testChannelID, bannedUserID))
	assert.Equal(t, "banned-abcdefgh", current.Username)
	assert.Equal(t, bannedNickname, current.Nickname)
	assert.Empty(t, current.FirstName)
	assert.Empty(t, current.LastName)
	assert.Equal(t, &kvstore.BannedIdentity{
		Username: "slur_name", FirstName: "Real", LastName: "Name", Nickname: "Nick",
		PlaceholderUsername: "banned-abcdefgh",
	}, kv.bans[bannedUserID].Identity)

	warning, err := p.unbanUser(sysAdminID, testChannelID, bannedUserID)
	assert.NoError(t, err)
	assert.Empty(t, warning)
	assert.Equal(t, original.Username, current.Username)
	assert.Equal(t, original.Nickname, current.Nickname)
	assert.Equal(t, original.FirstName, current.FirstName)
}

func TestRestoreIdentitySkipsChangedProfiles(t *testing.T) {
	p, api, _ := newTestPlugin(t, nil)
	api.On("GetUser", bannedUserID).Return(&model.User{Id: bannedUserID, Username: "renamed-by-admin"}, nil)

	warning := p.restoreBannedIdentity(bannedUserID, &kvstore.BannedIdentity{Username: "old", PlaceholderUsername: "banned-abcdefgh"})
	assert.Empty(t, warning)
	api.AssertNotCalled(t, "UpdateUser", mock.Anything)
}

func TestRestoreIdentityWarnsWhenUsernameTaken(t *testing.T) {
	p, api, _ := newTestPlugin(t, nil)
	api.On("GetUser", bannedUserID).Return(&model.User{Id: bannedUserID, Username: "banned-abcdefgh"}, nil)
	api.On("UpdateUser", mock.Anything).Return(nil, model.NewAppError("UpdateUser", "app.user.save.username_exists.app_error", nil, "", 400))
	api.On("LogWarn", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()

	warning := p.restoreBannedIdentity(bannedUserID, &kvstore.BannedIdentity{Username: "taken", PlaceholderUsername: "banned-abcdefgh"})
	assert.Contains(t, warning, "@taken")
}
