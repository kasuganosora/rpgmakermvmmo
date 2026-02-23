package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFullAuthLifecycle(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	username := UniqueID("auth")
	password := "testpass1234"

	// 1. First login → auto-registers, returns token.
	token1, accountID := ts.Login(t, username, password)
	require.NotEmpty(t, token1)
	require.Greater(t, accountID, int64(0))

	// 2. List characters → empty.
	resp := ts.Get(t, "/api/characters", token1)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var listResult map[string]interface{}
	ReadJSON(t, resp, &listResult)
	chars := listResult["characters"].([]interface{})
	assert.Empty(t, chars)

	// 3. Create character "Hero" class 1.
	charID := ts.CreateCharacter(t, token1, UniqueID("Hero"), 1)
	require.Greater(t, charID, int64(0))

	// 4. List characters → has 1 character.
	resp = ts.Get(t, "/api/characters", token1)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	ReadJSON(t, resp, &listResult)
	chars = listResult["characters"].([]interface{})
	assert.Len(t, chars, 1)

	// 5. Login again with same credentials → same account, new token.
	// Small delay to ensure different JWT timestamps.
	time.Sleep(1100 * time.Millisecond)
	token2, accountID2 := ts.Login(t, username, password)
	assert.Equal(t, accountID, accountID2)
	assert.NotEqual(t, token1, token2)

	// 6. New token should work.
	resp = ts.Get(t, "/api/characters", token2)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// 7. Logout using token2 → token2 invalidated.
	resp = ts.PostJSON(t, "/api/auth/logout", nil, token2)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// 8. Authenticated request with invalidated token → 401.
	resp = ts.Get(t, "/api/characters", token2)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()
}

func TestLoginWrongPassword(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	username := UniqueID("wrongpw")
	password := "correctpass"

	// Register.
	ts.Login(t, username, password)

	// Login with wrong password.
	resp := ts.PostJSON(t, "/api/auth/login", map[string]string{
		"username": username,
		"password": "wrongpassword",
	}, "")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()
}

func TestWSConnectionAuth(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// 1. Login → get token.
	token, _ := ts.Login(t, UniqueID("wsauth"), "pass1234")

	// 2. Connect WS with valid token → success.
	ws := ts.ConnectWS(t, token)
	ws.Close()

	// 3. Attempt WS connect with invalid token → should fail.
	dialer := websocket.Dialer{}
	_, resp, err := dialer.Dial(ts.WSURL+"?token=invalid-token-xxx", nil)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	assert.Error(t, err, "expected WS dial to fail with invalid token")

	// 4. Attempt WS connect with no token → should fail.
	_, resp, err = dialer.Dial(ts.WSURL, nil)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	assert.Error(t, err, "expected WS dial to fail with no token")
}

func TestTokenRefresh(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	token, _ := ts.Login(t, UniqueID("refresh"), "pass1234")

	// Small delay to ensure different JWT iat/exp timestamps (JWT uses second granularity).
	time.Sleep(1100 * time.Millisecond)

	// Refresh token.
	resp := ts.PostJSON(t, "/api/auth/refresh", nil, token)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var result map[string]interface{}
	ReadJSON(t, resp, &result)
	newToken := result["token"].(string)
	require.NotEmpty(t, newToken)
	assert.NotEqual(t, token, newToken)

	// Old token should no longer work.
	resp = ts.Get(t, "/api/characters", token)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	// New token should work.
	resp = ts.Get(t, "/api/characters", newToken)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

func TestCharacterLimit(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	token, _ := ts.Login(t, UniqueID("charlimit"), "pass1234")

	// Create 3 characters (maxCharacters = 3).
	for i := 0; i < 3; i++ {
		ts.CreateCharacter(t, token, UniqueID("Char"), 1)
	}

	// 4th character should fail.
	resp := ts.PostJSON(t, "/api/characters", map[string]interface{}{
		"name":       UniqueID("Extra"),
		"class_id":   1,
		"walk_name":  "Actor1",
		"walk_index": 0,
		"face_name":  "Actor1",
		"face_index": 0,
	}, token)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
}

func TestHealthEndpoint(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	resp := ts.Get(t, "/health", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var result map[string]interface{}
	ReadJSON(t, resp, &result)
	assert.Equal(t, "ok", result["status"])
}
