package integration

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGuildCreateAndJoin(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Player A creates a guild.
	tokenA, _ := ts.Login(t, UniqueID("gldA"), "pass1234")
	ts.CreateCharacter(t, tokenA, UniqueID("GuildLeader"), 1)

	guildName := UniqueID("TestGuild")
	resp := ts.PostJSON(t, "/api/guilds", map[string]interface{}{
		"name":   guildName,
		"notice": "Welcome!",
	}, tokenA)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var guildResult map[string]interface{}
	ReadJSON(t, resp, &guildResult)
	guildID := int64(guildResult["id"].(float64))
	require.Greater(t, guildID, int64(0))

	// Verify guild detail.
	resp = ts.Get(t, fmt.Sprintf("/api/guilds/%d", guildID), tokenA)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var detail map[string]interface{}
	ReadJSON(t, resp, &detail)
	guild := detail["guild"].(map[string]interface{})
	assert.Equal(t, guildName, guild["name"])
	members := detail["members"].([]interface{})
	assert.Len(t, members, 1) // Just the leader.

	// Player B joins the guild.
	tokenB, _ := ts.Login(t, UniqueID("gldB"), "pass1234")
	ts.CreateCharacter(t, tokenB, UniqueID("GuildMember"), 1)

	resp = ts.PostJSON(t, fmt.Sprintf("/api/guilds/%d/join", guildID), nil, tokenB)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Verify guild now has 2 members.
	resp = ts.Get(t, fmt.Sprintf("/api/guilds/%d", guildID), tokenA)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	ReadJSON(t, resp, &detail)
	members = detail["members"].([]interface{})
	assert.Len(t, members, 2)
}

func TestGuildUpdateNotice(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	tokenA, _ := ts.Login(t, UniqueID("notA"), "pass1234")
	ts.CreateCharacter(t, tokenA, UniqueID("NoticeLeader"), 1)

	resp := ts.PostJSON(t, "/api/guilds", map[string]interface{}{
		"name": UniqueID("NoticeGuild"),
	}, tokenA)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var guildResult map[string]interface{}
	ReadJSON(t, resp, &guildResult)
	guildID := int64(guildResult["id"].(float64))

	// Update notice.
	newNotice := "New notice content"
	resp = ts.Put(t, fmt.Sprintf("/api/guilds/%d/notice", guildID), map[string]interface{}{
		"notice": newNotice,
	}, tokenA)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Verify updated notice.
	resp = ts.Get(t, fmt.Sprintf("/api/guilds/%d", guildID), tokenA)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var detail map[string]interface{}
	ReadJSON(t, resp, &detail)
	guild := detail["guild"].(map[string]interface{})
	assert.Equal(t, newNotice, guild["notice"])
}

func TestFriendSystem(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Player A and Player B.
	tokenA, _ := ts.Login(t, UniqueID("frA"), "pass1234")
	charIDA := ts.CreateCharacter(t, tokenA, UniqueID("FriendA"), 1)

	tokenB, _ := ts.Login(t, UniqueID("frB"), "pass1234")
	charIDB := ts.CreateCharacter(t, tokenB, UniqueID("FriendB"), 1)

	// Player A sends friend request to Player B.
	resp := ts.PostJSON(t, "/api/social/friends/request", map[string]interface{}{
		"target_char_id": charIDB,
	}, tokenA)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// Player A's friends list should be empty (request is pending).
	resp = ts.Get(t, "/api/social/friends", tokenA)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var friendsResult map[string]interface{}
	ReadJSON(t, resp, &friendsResult)
	friends := friendsResult["friends"].([]interface{})
	assert.Empty(t, friends) // Pending, not yet accepted.

	// We need to find the friendship request ID.
	// Query the DB directly since there's no REST endpoint for pending requests.
	var requestID int64
	row := ts.DB.Raw("SELECT id FROM friendships WHERE char_id = ? AND friend_id = ? AND status = 0", charIDA, charIDB).Row()
	require.NoError(t, row.Scan(&requestID))
	require.Greater(t, requestID, int64(0))

	// Player B accepts the friend request.
	resp = ts.PostJSON(t, fmt.Sprintf("/api/social/friends/accept/%d", requestID), nil, tokenB)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Player A's friends list should now include B.
	resp = ts.Get(t, "/api/social/friends", tokenA)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	ReadJSON(t, resp, &friendsResult)
	friends = friendsResult["friends"].([]interface{})
	assert.Len(t, friends, 1)

	// Player B's friends list should include A.
	resp = ts.Get(t, "/api/social/friends", tokenB)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	ReadJSON(t, resp, &friendsResult)
	friends = friendsResult["friends"].([]interface{})
	assert.Len(t, friends, 1)
}

func TestGuildNonLeaderCannotUpdateNotice(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Leader creates guild.
	tokenA, _ := ts.Login(t, UniqueID("permA"), "pass1234")
	ts.CreateCharacter(t, tokenA, UniqueID("PermLeader"), 1)

	resp := ts.PostJSON(t, "/api/guilds", map[string]interface{}{
		"name": UniqueID("PermGuild"),
	}, tokenA)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var guildResult map[string]interface{}
	ReadJSON(t, resp, &guildResult)
	guildID := int64(guildResult["id"].(float64))

	// Non-leader joins.
	tokenB, _ := ts.Login(t, UniqueID("permB"), "pass1234")
	ts.CreateCharacter(t, tokenB, UniqueID("PermMember"), 1)

	resp = ts.PostJSON(t, fmt.Sprintf("/api/guilds/%d/join", guildID), nil, tokenB)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Non-leader tries to update notice → should be forbidden.
	resp = ts.Put(t, fmt.Sprintf("/api/guilds/%d/notice", guildID), map[string]interface{}{
		"notice": "Hacked!",
	}, tokenB)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp.Body.Close()
}
