package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	apirest "github.com/kasuganosora/rpgmakermvmmo/server/api/rest"
	"github.com/kasuganosora/rpgmakermvmmo/server/api/sse"
	apows "github.com/kasuganosora/rpgmakermvmmo/server/api/ws"
	"github.com/kasuganosora/rpgmakermvmmo/server/audit"
	"github.com/kasuganosora/rpgmakermvmmo/server/cache"
	"github.com/kasuganosora/rpgmakermvmmo/server/config"
	dbadapter "github.com/kasuganosora/rpgmakermvmmo/server/db"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/chat"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/party"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/quest"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/script"
	gskill "github.com/kasuganosora/rpgmakermvmmo/server/game/skill"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/trade"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/world"
	mw "github.com/kasuganosora/rpgmakermvmmo/server/middleware"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/kasuganosora/rpgmakermvmmo/server/scheduler"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

func main() {
	cfgPath := "config/config.yaml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// ---- Logger ----
	var logger *zap.Logger
	var logErr error
	if cfg.Server.Debug {
		logger, logErr = zap.NewDevelopment()
	} else {
		logger, logErr = zap.NewProduction()
	}
	if logErr != nil {
		log.Fatalf("logger: %v", logErr)
	}
	defer logger.Sync()

	// Warn loudly if admin endpoints will be disabled.
	if cfg.Server.AdminKey == "" {
		logger.Warn("server.admin_key is not set; admin endpoints are disabled")
	}

	// ---- Database ----
	db, err := dbadapter.Open(cfg.Database)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	if err := model.AutoMigrate(db); err != nil {
		log.Fatalf("db migrate: %v", err)
	}
	logger.Info("DB initialized")

	// ---- Audit ----
	auditSvc := audit.New(db, logger)
	defer auditSvc.Stop(nil)

	// ---- Cache / PubSub ----
	cacheConfig := cache.CacheConfig{
		RedisAddr:       cfg.Cache.RedisAddr,
		RedisPassword:   cfg.Cache.RedisPassword,
		RedisDB:         cfg.Cache.RedisDB,
		LocalGCInterval: cfg.Cache.LocalGCInterval,
		LocalPubSubBuf:  cfg.Cache.LocalPubSubBuf,
	}
	c, err := cache.NewCache(cacheConfig)
	if err != nil {
		log.Fatalf("cache: %v", err)
	}
	pubsub, err := cache.NewPubSub(cacheConfig)
	if err != nil {
		log.Fatalf("pubsub: %v", err)
	}
	logger.Info("Cache initialized")

	// ---- RMMV Resource Loader ----
	res := resource.NewLoader(cfg.RPGMaker.DataPath, cfg.RPGMaker.ImgPath)
	if err := res.Load(); err != nil {
		logger.Warn("resource load warning", zap.Error(err))
	} else {
		logger.Info("RMMV resources loaded")
	}

	// ---- Scheduler ----
	sched := scheduler.New(logger)
	defer sched.Stop()

	// ---- Game Systems ----
	sm := player.NewSessionManager(logger)
	gameState := world.NewGameState(db, logger)
	if err := gameState.LoadFromDB(); err != nil {
		logger.Warn("failed to load game state from DB", zap.Error(err))
	} else {
		logger.Info("game state loaded from DB")
	}
	whitelist := world.NewGlobalWhitelist()
	// Time/day/weather variables are shared so all players see the same time of day.
	for _, id := range []int{
		202, // ターン (day counter)
		203, // 曜日 (day of week)
		204, // 時間(h)
		205, // 時間(m)
		206, // 時間帯 (time period: dawn/morning/afternoon/evening/night)
		207, // 天候 (weather)
		211, // 時間表示 (formatted time display)
	} {
		whitelist.Variables[id] = true
	}
	for _, id := range []int{
		11,  // 時間切り替えフラグ (time switch flag)
		12,  // 時計オン (clock on)
		20,  // リアルタイム処理 (real-time processing)
		31,  // その場で時間経過 (time progression on spot)
		53,  // 日照 (sunlight)
		54,  // 日没 (sunset)
		55,  // 天候・雨 (rain)
		56,  // 天候・雲 (clouds)
		57,  // 天候・陽光 (sunny)
		58,  // 天候・瘴気 (miasma)
		87,  // 日数経過開始 (day count start)
		89,  // 平日 (weekday)
		103, // 時間経過オンオフ (time passage toggle)
		104, // 時間経過呼び出し (time passage call)
	} {
		whitelist.Switches[id] = true
	}
	wm := world.NewWorldManager(res, gameState, whitelist, db, logger)
	defer wm.StopAll()
	defer gameState.Stop()
	partyMgr := party.NewManager(logger)

	// ---- Services ----
	skillSvc := gskill.NewSkillService(c, res, wm, db, logger)
	chatH := chat.NewHandler(c, pubsub, sm, wm, cfg.Game, logger)
	tradeSvc := trade.NewService(db, c, sm, logger)
	questSvc := quest.NewService(db, nil, logger)
	_ = questSvc

	// ---- JS Sandbox ----
	sandbox := script.NewSandbox(cfg.Script.VMPoolSize, cfg.Script.Timeout, logger)
	_ = sandbox

	// ---- Periodic Scheduler Tasks ----
	sched.AddTicker("session_cleanup", 5*time.Minute, func() {
		// Session cleanup is passive (disconnect closes sessions); placeholder.
		logger.Debug("session cleanup tick")
	})
	sched.AddTicker("auto_save", time.Duration(cfg.Game.SaveIntervalS)*time.Second, func() {
		// Position auto-save is triggered per-session on tick; placeholder hook.
		logger.Debug("auto_save tick")
	})

	// ---- WS Router ----
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

	wsRouter.On("chat_send", chatH.HandleSend)

	// ---- Gin HTTP Server ----
	if !cfg.Server.Debug {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(mw.TraceID(), mw.Logger(logger), mw.Recovery(logger))
	r.Use(mw.RateLimit(rate.Limit(cfg.Security.RateLimitRPS), cfg.Security.RateLimitBurst))

	// Health check
	r.GET("/health", func(ctx *gin.Context) {
		ctx.JSON(200, gin.H{"status": "ok"})
	})

	// ---- Client plugin static files ----
	// Serves mmo-*.js files so the client loader can fetch them remotely.
	// Cache-Control: no-cache forces the browser to revalidate on every request,
	// so code changes are picked up immediately without clearing the browser cache.
	if cfg.Plugins.ClientDir != "" {
		r.Use(func(c *gin.Context) {
			if len(c.Request.URL.Path) > 9 && c.Request.URL.Path[:9] == "/plugins/" {
				c.Header("Cache-Control", "no-cache")
			}
			c.Next()
		})
		r.Static("/plugins", cfg.Plugins.ClientDir)
		logger.Info("Serving client plugins", zap.String("dir", cfg.Plugins.ClientDir))
	}

	// ---- REST API routes ----
	authH := apirest.NewAuthHandler(db, c, cfg.Security)
	charH := apirest.NewCharacterHandler(db, res, cfg.Game)
	invH := apirest.NewInventoryHandler(db)
	socialH := apirest.NewSocialHandler(db, sm)
	guildH := apirest.NewGuildHandler(db)
	mailH := apirest.NewMailHandler(db)
	rankH := apirest.NewRankingHandler(db, c, logger)
	shopH := apirest.NewShopHandler(db, res, nil)
	adminH := apirest.NewAdminHandler(db, sm, wm, sched, logger)

	// Ranking refresh scheduler task.
	_ = rankH // used below

	api := r.Group("/api")
	{
		authG := api.Group("/auth")
		authG.POST("/login", authH.Login)
		authG.POST("/logout", mw.Auth(cfg.Security, c), authH.Logout)
		authG.POST("/refresh", mw.Auth(cfg.Security, c), authH.Refresh)

		charsG := api.Group("/characters")
		charsG.Use(mw.Auth(cfg.Security, c))
		charsG.GET("", charH.List)
		charsG.POST("", charH.Create)
		charsG.DELETE("/:id", charH.Delete)
		charsG.GET("/:id/inventory", invH.List)
		charsG.GET("/:id/mail", mailH.List)
		charsG.POST("/:id/mail/:mail_id/claim", mailH.Claim)

		socialG := api.Group("/social")
		socialG.Use(mw.Auth(cfg.Security, c))
		socialG.GET("/friends", socialH.ListFriends)
		socialG.POST("/friends/request", socialH.SendFriendRequest)
		socialG.POST("/friends/accept/:id", socialH.AcceptFriendRequest)
		socialG.DELETE("/friends/:id", socialH.DeleteFriend)
		socialG.POST("/block/:id", socialH.BlockPlayer)

		guildsG := api.Group("/guilds")
		guildsG.Use(mw.Auth(cfg.Security, c))
		guildsG.POST("", guildH.Create)
		guildsG.GET("/:id", guildH.Detail)
		guildsG.POST("/:id/join", guildH.Join)
		guildsG.DELETE("/:id/members/:cid", guildH.KickMember)
		guildsG.PUT("/:id/notice", guildH.UpdateNotice)

		shopG := api.Group("/shop")
		shopG.Use(mw.Auth(cfg.Security, c))
		shopG.GET("/:id", shopH.Detail)
		shopG.POST("/:id/buy", shopH.Buy)
		shopG.POST("/:id/sell", shopH.Sell)

		// Client error reporting (no auth required — errors may happen before login).
		api.POST("/client-error", func(ctx *gin.Context) {
			var body struct {
				Message string `json:"message"`
				Source  string `json:"source"`
				Line    int    `json:"line"`
				Col     int    `json:"col"`
				Stack   string `json:"stack"`
				UA      string `json:"ua"`
			}
			if err := ctx.ShouldBindJSON(&body); err != nil {
				ctx.JSON(400, gin.H{"error": "bad request"})
				return
			}
			logger.Error("CLIENT ERROR",
				zap.String("message", body.Message),
				zap.String("source", body.Source),
				zap.Int("line", body.Line),
				zap.Int("col", body.Col),
				zap.String("stack", body.Stack),
				zap.String("ua", body.UA),
			)
			ctx.JSON(200, gin.H{"status": "received"})
		})

		rankG := api.Group("/ranking")
		rankG.GET("/exp", rankH.TopExp)

		adminG := api.Group("/admin")
		adminG.Use(apirest.AdminAuth(cfg.Server.AdminKey))
		adminG.GET("/metrics", adminH.Metrics)
		adminG.GET("/players", adminH.ListPlayers)
		adminG.POST("/kick/:id", adminH.KickPlayer)
		adminG.POST("/accounts/:id/ban", adminH.BanAccount)
		adminG.GET("/scheduler", adminH.ListSchedulerTasks)
	}

	_ = auditSvc

	// ---- WebSocket ----
	wsH := apows.NewHandler(db, c, cfg.Security, sm, wm, partyMgr, tradeSvc, wsRouter, logger)
	r.GET("/ws", wsH.ServeWS)

	// ---- SSE ----
	sseH := sse.NewHandler(pubsub, c, cfg.Security, logger)
	r.GET("/sse", sseH.ServeSSE)

	// ---- RMMV game static files (browser client) ----
	// Serves the game's www/ directory so players can open the game in a browser.
	// Enables multi-client testing by opening multiple browser tabs.
	if cfg.Server.GameDir != "" {
		r.Static("/game-assets", cfg.Server.GameDir)
		// Serve index.html at root for convenience.
		r.StaticFile("/", cfg.Server.GameDir+"/index.html")
		// NoRoute fallback: try to serve from game dir (for js/, data/, img/, audio/, fonts/).
		r.NoRoute(func(c *gin.Context) {
			path := cfg.Server.GameDir + c.Request.URL.Path
			if _, err := os.Stat(path); err == nil {
				c.File(path)
				return
			}
			c.JSON(404, gin.H{"error": "not found"})
		})
		logger.Info("Serving RMMV game files", zap.String("dir", cfg.Server.GameDir))
	}

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	logger.Info("Server listening", zap.String("addr", addr))
	if err := r.Run(addr); err != nil {
		log.Fatalf("server: %v", err)
	}
}
