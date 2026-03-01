package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	apirest "github.com/kasuganosora/rpgmakermvmmo/server/api/rest"
	apows "github.com/kasuganosora/rpgmakermvmmo/server/api/ws"
	"github.com/kasuganosora/rpgmakermvmmo/server/cache"
	"github.com/kasuganosora/rpgmakermvmmo/server/config"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/chat"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/party"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/quest"
	gskill "github.com/kasuganosora/rpgmakermvmmo/server/game/skill"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/trade"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/world"
	mw "github.com/kasuganosora/rpgmakermvmmo/server/middleware"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/kasuganosora/rpgmakermvmmo/server/scheduler"
	"github.com/kasuganosora/rpgmakermvmmo/server/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
	"gorm.io/gorm"
)

// TestServer wraps a real HTTP server with all MMO subsystems wired together.
type TestServer struct {
	DB        *gorm.DB
	Cache     cache.Cache
	PubSub    cache.PubSub
	SM        *player.SessionManager
	WM        *world.WorldManager
	Res       *resource.ResourceLoader
	BattleMgr *apows.BattleSessionManager
	Server    *httptest.Server
	URL       string // http://127.0.0.1:<port>
	WSURL     string // ws://127.0.0.1:<port>/ws
	Sec       config.SecurityConfig
}

// NewTestServer creates a fully wired MMO server for integration testing.
// It mirrors the dependency wiring in main.go.
func NewTestServer(t *testing.T) *TestServer {
	t.Helper()
	gin.SetMode(gin.TestMode)

	// ---- Infrastructure ----
	db := testutil.SetupTestDB(t)
	c, pubsub := testutil.SetupTestCache(t)
	logger := zap.NewNop()

	sec := config.SecurityConfig{
		JWTSecret:      "integration-test-secret",
		JWTTTLH:        72 * time.Hour,
		RateLimitRPS:   1000,
		RateLimitBurst: 2000,
		AllowedOrigins: []string{}, // allow all origins
	}

	// ---- Game Systems ----
	sm := player.NewSessionManager(logger)

	// Empty resource loader (no RMMV data files needed for integration tests).
	res := resource.NewLoader("", "")
	gameState := world.NewGameState(nil, logger)
	wm := world.NewWorldManager(res, gameState, world.NewGlobalWhitelist(), nil, logger)

	// ---- Services ----
	skillSvc := gskill.NewSkillService(c, res, wm, db, logger)
	chatH := chat.NewHandler(c, pubsub, sm, wm, config.GameConfig{
		ChatNearbyRange:     10,
		GlobalChatCooldownS: 180,
	}, logger)
	tradeSvc := trade.NewService(db, c, sm, logger)
	partyMgr := party.NewManager(logger)
	questSvc := quest.NewService(db, nil, logger)
	_ = questSvc

	sched := scheduler.New(logger)

	// ---- WS Router ----
	wsRouter := apows.NewRouter(logger)
	gh := apows.NewGameHandlers(db, wm, sm, res, logger)
	gh.RegisterHandlers(wsRouter)

	bh := apows.NewBattleHandlers(db, wm, res, logger)
	bh.RegisterHandlers(wsRouter)

	sh := apows.NewSkillItemHandlers(db, res, wm, skillSvc, logger)
	sh.RegisterHandlers(wsRouter)

	npcH := apows.NewNPCHandlers(db, res, wm, logger)
	npcH.RegisterHandlers(wsRouter)

	tradeH := apows.NewTradeHandlers(db, tradeSvc, sm, logger)
	tradeH.RegisterHandlers(wsRouter)

	partyH := apows.NewPartyHandlers(partyMgr, sm, logger)
	partyH.RegisterHandlers(wsRouter)

	// TemplateEvent.js hook handlers
	templateEventH := apows.NewTemplateEventHandlers(db, wm, sm, logger)
	templateEventH.RegisterHandlers(wsRouter)

	wsRouter.On("chat_send", chatH.HandleSend)

	// ---- Gin HTTP Server ----
	r := gin.New()
	r.Use(mw.TraceID(), mw.Recovery(logger))
	r.Use(mw.RateLimit(rate.Limit(sec.RateLimitRPS), sec.RateLimitBurst))

	r.GET("/health", func(ctx *gin.Context) {
		ctx.JSON(200, gin.H{"status": "ok"})
	})

	// ---- REST API routes (mirrors main.go) ----
	authH := apirest.NewAuthHandler(db, c, sec)
	charH := apirest.NewCharacterHandler(db, nil, config.GameConfig{StartMapID: 1, StartX: 5, StartY: 5})
	invH := apirest.NewInventoryHandler(db)
	socialH := apirest.NewSocialHandler(db, sm)
	guildH := apirest.NewGuildHandler(db)
	mailH := apirest.NewMailHandler(db)
	rankH := apirest.NewRankingHandler(db, c, logger)
	shopH := apirest.NewShopHandler(db, res, nil)
	adminH := apirest.NewAdminHandler(db, sm, wm, sched, logger)
	_ = rankH
	_ = adminH

	api := r.Group("/api")
	{
		authG := api.Group("/auth")
		authG.POST("/login", authH.Login)
		authG.POST("/logout", mw.Auth(sec, c), authH.Logout)
		authG.POST("/refresh", mw.Auth(sec, c), authH.Refresh)

		charsG := api.Group("/characters")
		charsG.Use(mw.Auth(sec, c))
		charsG.GET("", charH.List)
		charsG.POST("", charH.Create)
		charsG.DELETE("/:id", charH.Delete)
		charsG.GET("/:id/inventory", invH.List)
		charsG.GET("/:id/mail", mailH.List)
		charsG.POST("/:id/mail/:mail_id/claim", mailH.Claim)

		socialG := api.Group("/social")
		socialG.Use(mw.Auth(sec, c))
		socialG.GET("/friends", socialH.ListFriends)
		socialG.POST("/friends/request", socialH.SendFriendRequest)
		socialG.POST("/friends/accept/:id", socialH.AcceptFriendRequest)
		socialG.DELETE("/friends/:id", socialH.DeleteFriend)
		socialG.POST("/block/:id", socialH.BlockPlayer)

		guildsG := api.Group("/guilds")
		guildsG.Use(mw.Auth(sec, c))
		guildsG.POST("", guildH.Create)
		guildsG.GET("/:id", guildH.Detail)
		guildsG.POST("/:id/join", guildH.Join)
		guildsG.DELETE("/:id/members/:cid", guildH.KickMember)
		guildsG.PUT("/:id/notice", guildH.UpdateNotice)

		shopG := api.Group("/shop")
		shopG.Use(mw.Auth(sec, c))
		shopG.GET("/:id", shopH.Detail)
		shopG.POST("/:id/buy", shopH.Buy)
		shopG.POST("/:id/sell", shopH.Sell)

		rankG := api.Group("/ranking")
		rankG.GET("/exp", rankH.TopExp)
	}

	// ---- WebSocket ----
	wsH := apows.NewHandler(db, c, sec, sm, wm, partyMgr, tradeSvc, wsRouter, logger)
	r.GET("/ws", wsH.ServeWS)

	// ---- Start server ----
	server := httptest.NewServer(r)
	url := server.URL
	wsURL := "ws" + url[len("http"):] + "/ws"

	return &TestServer{
		DB:     db,
		Cache:  c,
		PubSub: pubsub,
		SM:     sm,
		WM:     wm,
		Res:    res,
		Server: server,
		URL:    url,
		WSURL:  wsURL,
		Sec:    sec,
	}
}

// NewTestServerWithResources creates a test server that loads real RMMV data
// from the given dataPath (e.g., projectb/www/data).
func NewTestServerWithResources(t *testing.T, dataPath string) *TestServer {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db := testutil.SetupTestDB(t)
	c, pubsub := testutil.SetupTestCache(t)
	logger := zap.NewNop()

	sec := config.SecurityConfig{
		JWTSecret:      "integration-test-secret",
		JWTTTLH:        72 * time.Hour,
		RateLimitRPS:   1000,
		RateLimitBurst: 2000,
		AllowedOrigins: []string{},
	}

	sm := player.NewSessionManager(logger)

	// Load real RMMV resources.
	res := resource.NewLoader(dataPath, "")
	err := res.Load()
	require.NoError(t, err, "Failed to load RMMV resources from %s", dataPath)

	gameState := world.NewGameState(nil, logger)
	wm := world.NewWorldManager(res, gameState, world.NewGlobalWhitelist(), nil, logger)

	skillSvc := gskill.NewSkillService(c, res, wm, db, logger)
	chatH := chat.NewHandler(c, pubsub, sm, wm, config.GameConfig{
		ChatNearbyRange:     10,
		GlobalChatCooldownS: 180,
	}, logger)
	tradeSvc := trade.NewService(db, c, sm, logger)
	partyMgr := party.NewManager(logger)
	questSvc := quest.NewService(db, nil, logger)
	_ = questSvc

	sched := scheduler.New(logger)

	// WS Router
	wsRouter := apows.NewRouter(logger)
	gh := apows.NewGameHandlers(db, wm, sm, res, logger)
	gh.RegisterHandlers(wsRouter)

	bh := apows.NewBattleHandlers(db, wm, res, logger)
	bh.RegisterHandlers(wsRouter)

	sh := apows.NewSkillItemHandlers(db, res, wm, skillSvc, logger)
	sh.RegisterHandlers(wsRouter)

	npcH := apows.NewNPCHandlers(db, res, wm, logger)
	npcH.SetTransferFunc(gh.EnterMapRoom)
	npcH.RegisterHandlers(wsRouter)
	gh.SetAutorunFunc(npcH.ExecuteAutoruns)

	tradeH := apows.NewTradeHandlers(db, tradeSvc, sm, logger)
	tradeH.RegisterHandlers(wsRouter)

	partyH := apows.NewPartyHandlers(partyMgr, sm, logger)
	partyH.RegisterHandlers(wsRouter)

	templateEventH := apows.NewTemplateEventHandlers(db, wm, sm, logger)
	templateEventH.RegisterHandlers(wsRouter)

	// Battle session manager
	battleMgr := apows.NewBattleSessionManager(res, partyMgr, logger)
	battleMgr.RegisterHandlers(wsRouter)

	// Debug handlers
	debugH := apows.NewDebugHandlers(wm, sm, res, db, logger)
	debugH.SetTransferFunc(gh.EnterMapRoom)
	debugH.SetBattleFn(battleMgr.RunBattle)
	debugH.RegisterHandlers(wsRouter)

	wsRouter.On("chat_send", chatH.HandleSend)

	// Gin HTTP Server
	r := gin.New()
	r.Use(mw.TraceID(), mw.Recovery(logger))
	r.Use(mw.RateLimit(rate.Limit(sec.RateLimitRPS), sec.RateLimitBurst))

	r.GET("/health", func(ctx *gin.Context) {
		ctx.JSON(200, gin.H{"status": "ok"})
	})

	authH := apirest.NewAuthHandler(db, c, sec)
	charH := apirest.NewCharacterHandler(db, nil, config.GameConfig{StartMapID: 1, StartX: 5, StartY: 5})
	invH := apirest.NewInventoryHandler(db)
	socialH := apirest.NewSocialHandler(db, sm)
	guildH := apirest.NewGuildHandler(db)
	mailH := apirest.NewMailHandler(db)
	rankH := apirest.NewRankingHandler(db, c, logger)
	shopH := apirest.NewShopHandler(db, res, nil)
	adminH := apirest.NewAdminHandler(db, sm, wm, sched, logger)
	_ = rankH
	_ = adminH

	api := r.Group("/api")
	{
		authG := api.Group("/auth")
		authG.POST("/login", authH.Login)
		authG.POST("/logout", mw.Auth(sec, c), authH.Logout)
		authG.POST("/refresh", mw.Auth(sec, c), authH.Refresh)

		charsG := api.Group("/characters")
		charsG.Use(mw.Auth(sec, c))
		charsG.GET("", charH.List)
		charsG.POST("", charH.Create)
		charsG.DELETE("/:id", charH.Delete)
		charsG.GET("/:id/inventory", invH.List)
		charsG.GET("/:id/mail", mailH.List)
		charsG.POST("/:id/mail/:mail_id/claim", mailH.Claim)

		socialG := api.Group("/social")
		socialG.Use(mw.Auth(sec, c))
		socialG.GET("/friends", socialH.ListFriends)
		socialG.POST("/friends/request", socialH.SendFriendRequest)
		socialG.POST("/friends/accept/:id", socialH.AcceptFriendRequest)
		socialG.DELETE("/friends/:id", socialH.DeleteFriend)
		socialG.POST("/block/:id", socialH.BlockPlayer)

		guildsG := api.Group("/guilds")
		guildsG.Use(mw.Auth(sec, c))
		guildsG.POST("", guildH.Create)
		guildsG.GET("/:id", guildH.Detail)
		guildsG.POST("/:id/join", guildH.Join)
		guildsG.DELETE("/:id/members/:cid", guildH.KickMember)
		guildsG.PUT("/:id/notice", guildH.UpdateNotice)

		shopG := api.Group("/shop")
		shopG.Use(mw.Auth(sec, c))
		shopG.GET("/:id", shopH.Detail)
		shopG.POST("/:id/buy", shopH.Buy)
		shopG.POST("/:id/sell", shopH.Sell)

		rankG := api.Group("/ranking")
		rankG.GET("/exp", rankH.TopExp)
	}

	wsH := apows.NewHandler(db, c, sec, sm, wm, partyMgr, tradeSvc, wsRouter, logger)
	r.GET("/ws", wsH.ServeWS)

	server := httptest.NewServer(r)
	url := server.URL
	wsURL := "ws" + url[len("http"):] + "/ws"

	return &TestServer{
		DB:        db,
		Cache:     c,
		PubSub:    pubsub,
		SM:        sm,
		WM:        wm,
		Res:       res,
		BattleMgr: battleMgr,
		Server:    server,
		URL:       url,
		WSURL:     wsURL,
		Sec:       sec,
	}
}

// Close shuts down the test server and all game systems.
func (ts *TestServer) Close() {
	ts.WM.StopAll()
	ts.Server.Close()
}

// --- HTTP helpers ---

// PostJSON sends a POST request with JSON body and optional Bearer token.
func (ts *TestServer) PostJSON(t *testing.T, path string, body interface{}, token string) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest("POST", ts.URL+path, bytes.NewReader(data))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// Get sends a GET request with optional Bearer token.
func (ts *TestServer) Get(t *testing.T, path string, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest("GET", ts.URL+path, nil)
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// Delete sends a DELETE request with JSON body and optional Bearer token.
func (ts *TestServer) Delete(t *testing.T, path string, body interface{}, token string) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		require.NoError(t, err)
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequest("DELETE", ts.URL+path, bodyReader)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// Put sends a PUT request with JSON body and optional Bearer token.
func (ts *TestServer) Put(t *testing.T, path string, body interface{}, token string) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest("PUT", ts.URL+path, bytes.NewReader(data))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// ReadJSON reads and decodes a JSON response body into the given target.
func ReadJSON(t *testing.T, resp *http.Response, target interface{}) {
	t.Helper()
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, target), "body: %s", string(data))
}

// --- Auth helpers ---

// Login logs in (auto-registers on first call) and returns the token and account ID.
func (ts *TestServer) Login(t *testing.T, username, password string) (token string, accountID int64) {
	t.Helper()
	resp := ts.PostJSON(t, "/api/auth/login", map[string]string{
		"username": username,
		"password": password,
	}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var result map[string]interface{}
	ReadJSON(t, resp, &result)
	token = result["token"].(string)
	accountID = int64(result["account_id"].(float64))
	return
}

// CreateCharacter creates a character and returns its ID.
func (ts *TestServer) CreateCharacter(t *testing.T, token, name string, classID int) int64 {
	t.Helper()
	resp := ts.PostJSON(t, "/api/characters", map[string]interface{}{
		"name":       name,
		"class_id":   classID,
		"walk_name":  "Actor1",
		"walk_index": 0,
		"face_name":  "Actor1",
		"face_index": 0,
	}, token)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var result map[string]interface{}
	ReadJSON(t, resp, &result)
	return int64(result["id"].(float64))
}

// --- WebSocket client ---

// WSClient wraps a gorilla/websocket connection for integration testing.
// Uses a background readLoop to avoid gorilla/websocket's SetReadDeadline bug.
type WSClient struct {
	Conn   *websocket.Conn
	t      *testing.T
	seq    uint64
	readCh chan readResult // buffered channel from readLoop
}

type readResult struct {
	data []byte
	err  error
}

// ConnectWS dials the test server's WS endpoint with the given JWT token.
func (ts *TestServer) ConnectWS(t *testing.T, token string) *WSClient {
	t.Helper()
	url := ts.WSURL + "?token=" + token
	dialer := websocket.Dialer{}
	conn, resp, err := dialer.Dial(url, nil)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	require.NoError(t, err, "WS dial failed")
	wc := &WSClient{Conn: conn, t: t, readCh: make(chan readResult, 256)}
	go wc.readLoop()
	return wc
}

// readLoop continuously reads from the websocket in a dedicated goroutine.
func (wc *WSClient) readLoop() {
	for {
		_, data, err := wc.Conn.ReadMessage()
		wc.readCh <- readResult{data, err}
		if err != nil {
			return
		}
	}
}

// Send writes a JSON message packet to the WebSocket.
func (wc *WSClient) Send(msgType string, payload interface{}) {
	wc.t.Helper()
	seq := atomic.AddUint64(&wc.seq, 1)
	payloadJSON, err := json.Marshal(payload)
	require.NoError(wc.t, err)
	pkt := map[string]interface{}{
		"seq":     seq,
		"type":    msgType,
		"payload": json.RawMessage(payloadJSON),
	}
	data, err := json.Marshal(pkt)
	require.NoError(wc.t, err)
	require.NoError(wc.t, wc.Conn.WriteMessage(websocket.TextMessage, data))
}

// Recv reads one message from the WebSocket with a timeout.
func (wc *WSClient) Recv(timeout time.Duration) map[string]interface{} {
	wc.t.Helper()
	pkt, err := wc.RecvAny(timeout)
	require.NoError(wc.t, err, "WS recv failed")
	return pkt
}

// RecvAny reads one message from the WebSocket with a timeout, returning an error
// instead of failing the test on timeout/read failure.
// Reads from the background readLoop channel to avoid gorilla/websocket's
// SetReadDeadline bug which permanently corrupts the connection after a timeout.
func (wc *WSClient) RecvAny(timeout time.Duration) (map[string]interface{}, error) {
	select {
	case res := <-wc.readCh:
		if res.err != nil {
			return nil, res.err
		}
		var pkt map[string]interface{}
		if err := json.Unmarshal(res.data, &pkt); err != nil {
			return nil, err
		}
		if payloadStr, ok := pkt["payload"].(string); ok {
			var nested interface{}
			if json.Unmarshal([]byte(payloadStr), &nested) == nil {
				pkt["payload"] = nested
			}
		}
		return pkt, nil
	case <-time.After(timeout):
		return nil, &timeoutError{}
	}
}

// timeoutError implements net.Error for timeout detection in callers.
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "read timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

// RecvType reads messages until one with the given type is found (within timeout).
func (wc *WSClient) RecvType(msgType string, timeout time.Duration) map[string]interface{} {
	wc.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		pkt, err := wc.RecvAny(remaining)
		if err != nil {
			wc.t.Fatalf("WS recv failed while waiting for %q: %v", msgType, err)
		}
		if pkt["type"] == msgType {
			return pkt
		}
	}
	wc.t.Fatalf("timed out waiting for message type %q", msgType)
	return nil
}

// Close closes the WebSocket connection.
func (wc *WSClient) Close() {
	_ = wc.Conn.Close()
}

// PayloadMap extracts the payload from a received WS packet as a map.
func PayloadMap(t *testing.T, pkt map[string]interface{}) map[string]interface{} {
	t.Helper()
	p := pkt["payload"]
	if p == nil {
		return map[string]interface{}{}
	}
	switch v := p.(type) {
	case map[string]interface{}:
		return v
	case string:
		var m map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(v), &m))
		return m
	default:
		// Try re-marshal + unmarshal for json.RawMessage etc.
		data, err := json.Marshal(v)
		require.NoError(t, err)
		var m map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &m))
		return m
	}
}

// --- Composite helper ---

// LoginAndConnect performs login, character creation, WS connect, and enter_map.
// Returns token, charID, and connected WSClient.
func (ts *TestServer) LoginAndConnect(t *testing.T, username, charName string) (string, int64, *WSClient) {
	t.Helper()
	token, _ := ts.Login(t, username, username+"pass")
	charID := ts.CreateCharacter(t, token, charName, 1)
	ws := ts.ConnectWS(t, token)
	ws.Send("enter_map", map[string]interface{}{"char_id": charID})
	// Wait for map_init response.
	pkt := ws.RecvType("map_init", 5*time.Second)
	require.NotNil(t, pkt)
	// Small delay to let the session fully register.
	time.Sleep(50 * time.Millisecond)
	return token, charID, ws
}

// UniqueID returns a short unique string suitable for usernames/character names.
var testCounter uint64

func UniqueID(prefix string) string {
	n := atomic.AddUint64(&testCounter, 1)
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano()%100000, n)
}

// --- Debug helpers ---

// DebugSetSwitch sends a debug_set_switch WS message and waits for debug_ok.
func (wc *WSClient) DebugSetSwitch(t *testing.T, switchID int, value bool) {
	t.Helper()
	wc.Send("debug_set_switch", map[string]interface{}{"switch_id": switchID, "value": value})
	wc.RecvType("debug_ok", 5*time.Second)
}

// DebugSetVariable sends a debug_set_variable WS message and waits for debug_ok.
func (wc *WSClient) DebugSetVariable(t *testing.T, variableID int, value int) {
	t.Helper()
	wc.Send("debug_set_variable", map[string]interface{}{"variable_id": variableID, "value": value})
	wc.RecvType("debug_ok", 5*time.Second)
}

// DebugGetState sends debug_get_state and returns the payload map.
func (wc *WSClient) DebugGetState(t *testing.T) map[string]interface{} {
	t.Helper()
	wc.Send("debug_get_state", map[string]interface{}{})
	pkt := wc.RecvType("debug_state", 5*time.Second)
	return PayloadMap(t, pkt)
}

// DebugTeleport sends a debug_teleport WS message and waits for debug_ok.
func (wc *WSClient) DebugTeleport(t *testing.T, mapID, x, y, dir int) {
	t.Helper()
	wc.Send("debug_teleport", map[string]interface{}{"map_id": mapID, "x": x, "y": y, "dir": dir})
	wc.RecvType("debug_ok", 5*time.Second)
}

// DebugSetStats sends a debug_set_stats WS message and waits for debug_ok.
func (wc *WSClient) DebugSetStats(t *testing.T, stats map[string]interface{}) {
	t.Helper()
	wc.Send("debug_set_stats", stats)
	wc.RecvType("debug_ok", 5*time.Second)
}

// SetPosition directly sets a player's position on the server.
func (ts *TestServer) SetPosition(t *testing.T, charID int64, x, y, dir int) {
	t.Helper()
	sess := ts.SM.Get(charID)
	require.NotNil(t, sess, "session not found for charID %d", charID)
	sess.SetPosition(x, y, dir)
}

// SetVariable directly sets a player variable on the server.
func (ts *TestServer) SetVariable(t *testing.T, charID int64, variableID, value int) {
	t.Helper()
	ps, err := ts.WM.PlayerStateManager().GetOrLoad(charID)
	require.NoError(t, err, "failed to load player state for charID %d", charID)
	ps.SetVariable(variableID, value)
}
