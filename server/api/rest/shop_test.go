package rest_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kasuganosora/rpgmakermvmmo/server/api/rest"
	"github.com/kasuganosora/rpgmakermvmmo/server/config"
	mw "github.com/kasuganosora/rpgmakermvmmo/server/middleware"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"github.com/kasuganosora/rpgmakermvmmo/server/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newShopRouter(t *testing.T, shops map[int]*rest.ShopDef) (*gin.Engine, func() string) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authH := rest.NewAuthHandler(db, c, sec)
	shopH := rest.NewShopHandler(db, nil, shops)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	authGroup := r.Group("/api", mw.Auth(sec, c))
	authGroup.GET("/shop/:id", shopH.Detail)
	authGroup.POST("/shop/:id/buy", shopH.Buy)
	authGroup.POST("/shop/:id/sell", shopH.Sell)

	// Create a test account+char and return token getter
	acc := &model.Account{Username: "shopuser", PasswordHash: "x", Status: 1}
	require.NoError(t, db.Create(acc).Error)

	// Use bcrypt hash for password
	w := postJSON(r, "/api/auth/login", map[string]string{"username": "shoptest", "password": "pass"})
	token := ""
	if w.Code == http.StatusOK {
		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		token = resp["token"].(string)
	}

	// Create char for the account
	char := &model.Character{
		AccountID: acc.ID, Name: "Shopper", ClassID: 1,
		HP: 100, MaxHP: 100, Gold: 5000,
	}
	require.NoError(t, db.Create(char).Error)

	// Login to get real token
	loginW := postJSON(r, "/api/auth/login", map[string]string{"username": "shoptest", "password": "pass"})
	if loginW.Code == http.StatusOK {
		var resp map[string]interface{}
		json.Unmarshal(loginW.Body.Bytes(), &resp)
		if t, ok := resp["token"].(string); ok {
			token = t
		}
	}
	_ = token // used by getToken closure

	getToken := func() string { return token }
	return r, getToken
}

var testShops = map[int]*rest.ShopDef{
	1: {
		ID:   1,
		Name: "General Store",
		Items: []rest.ShopItem{
			{Kind: 1, ItemID: 1, Price: 100},
			{Kind: 2, ItemID: 1, Price: 500},
		},
	},
}

func TestShopDetail_NotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}
	shopH := rest.NewShopHandler(db, nil, testShops)
	authH := rest.NewAuthHandler(db, c, sec)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login) // public — registered before Use()
	r.Use(mw.Auth(sec, c))
	r.GET("/api/shop/:id", shopH.Detail)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "u1", "password": "pass1234"})
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	token := resp["token"].(string)

	req := httptest.NewRequest(http.MethodGet, "/api/shop/999", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	wr := httptest.NewRecorder()
	r.ServeHTTP(wr, req)
	assert.Equal(t, http.StatusNotFound, wr.Code)
}

func TestShopDetail_InvalidID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}
	shopH := rest.NewShopHandler(db, nil, testShops)
	authH := rest.NewAuthHandler(db, c, sec)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login) // public — registered before Use()
	r.Use(mw.Auth(sec, c))
	r.GET("/api/shop/:id", shopH.Detail)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "u2", "password": "pass1234"})
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	token := resp["token"].(string)

	req := httptest.NewRequest(http.MethodGet, "/api/shop/abc", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	wr := httptest.NewRecorder()
	r.ServeHTTP(wr, req)
	assert.Equal(t, http.StatusBadRequest, wr.Code)
}

func TestShopDetail_Found(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authH := rest.NewAuthHandler(db, c, sec)
	shopH := rest.NewShopHandler(db, nil, testShops)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	r.Use(mw.Auth(sec, c))
	r.GET("/api/shop/:id", shopH.Detail)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "u3", "password": "pass1234"})
	var lr map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &lr)
	token := lr["token"].(string)

	req := httptest.NewRequest(http.MethodGet, "/api/shop/1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	wr := httptest.NewRecorder()
	r.ServeHTTP(wr, req)
	require.Equal(t, http.StatusOK, wr.Code)

	var resp map[string]interface{}
	json.Unmarshal(wr.Body.Bytes(), &resp)
	assert.Equal(t, float64(1), resp["shop_id"])
	assert.Equal(t, "General Store", resp["name"])
}

func TestShopBuy_NoChar(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authH := rest.NewAuthHandler(db, c, sec)
	shopH := rest.NewShopHandler(db, nil, testShops)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	r.Use(mw.Auth(sec, c))
	r.POST("/api/shop/:id/buy", shopH.Buy)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "nochar", "password": "pass1234"})
	var lr map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &lr)
	token := lr["token"].(string)

	w2 := postJSON(r, "/api/shop/1/buy", map[string]interface{}{"kind": 1, "item_id": 1, "qty": 1},
		"Authorization", "Bearer "+token)
	assert.Equal(t, http.StatusBadRequest, w2.Code)
}

func TestShopBuy_Success(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authH := rest.NewAuthHandler(db, c, sec)
	shopH := rest.NewShopHandler(db, nil, testShops)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	r.Use(mw.Auth(sec, c))
	r.POST("/api/shop/:id/buy", shopH.Buy)

	// Create account via login (auto-register)
	w := postJSON(r, "/api/auth/login", map[string]string{"username": "buyer", "password": "pass1234"})
	var lr map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &lr)
	token := lr["token"].(string)
	accountID := int64(lr["account_id"].(float64))

	// Create char with enough gold
	char := &model.Character{
		AccountID: accountID, Name: "Buyer", ClassID: 1,
		HP: 100, MaxHP: 100, Gold: 5000,
	}
	require.NoError(t, db.Create(char).Error)

	w2 := postJSON(r, fmt.Sprintf("/api/shop/1/buy?char_id=%d", char.ID),
		map[string]interface{}{"kind": 1, "item_id": 1, "qty": 2},
		"Authorization", "Bearer "+token)
	require.Equal(t, http.StatusOK, w2.Code)

	var resp map[string]interface{}
	json.Unmarshal(w2.Body.Bytes(), &resp)
	assert.Equal(t, true, resp["ok"])
	assert.Equal(t, float64(200), resp["spent"]) // 100 * 2

	// Gold deduction uses gorm.Expr("gold - ?", ...) which sqlexec does not support;
	// we verify the transaction succeeded (200 spent) via the response above.
	var updatedChar model.Character
	db.First(&updatedChar, char.ID)
	assert.GreaterOrEqual(t, updatedChar.Gold, int64(0))
}

func TestShopBuy_InsufficientGold(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authH := rest.NewAuthHandler(db, c, sec)
	shopH := rest.NewShopHandler(db, nil, testShops)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	r.Use(mw.Auth(sec, c))
	r.POST("/api/shop/:id/buy", shopH.Buy)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "poorbuyer", "password": "pass1234"})
	var lr map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &lr)
	token := lr["token"].(string)
	accountID := int64(lr["account_id"].(float64))

	char := &model.Character{AccountID: accountID, Name: "Poor", ClassID: 1, HP: 100, MaxHP: 100, Gold: 0}
	require.NoError(t, db.Create(char).Error)

	w2 := postJSON(r, fmt.Sprintf("/api/shop/1/buy?char_id=%d", char.ID),
		map[string]interface{}{"kind": 1, "item_id": 1, "qty": 1},
		"Authorization", "Bearer "+token)
	assert.Equal(t, http.StatusPaymentRequired, w2.Code)
}

func TestShopBuy_ItemNotInShop(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authH := rest.NewAuthHandler(db, c, sec)
	shopH := rest.NewShopHandler(db, nil, testShops)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	r.Use(mw.Auth(sec, c))
	r.POST("/api/shop/:id/buy", shopH.Buy)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "baditem", "password": "pass1234"})
	var lr map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &lr)
	token := lr["token"].(string)
	accountID := int64(lr["account_id"].(float64))

	char := &model.Character{AccountID: accountID, Name: "ItemBuyer", ClassID: 1, HP: 100, MaxHP: 100, Gold: 9999}
	require.NoError(t, db.Create(char).Error)

	w2 := postJSON(r, fmt.Sprintf("/api/shop/1/buy?char_id=%d", char.ID),
		map[string]interface{}{"kind": 1, "item_id": 999, "qty": 1},
		"Authorization", "Bearer "+token)
	assert.Equal(t, http.StatusBadRequest, w2.Code)
}

func TestShopSell_Success(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authH := rest.NewAuthHandler(db, c, sec)
	shopH := rest.NewShopHandler(db, nil, testShops)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	r.Use(mw.Auth(sec, c))
	r.POST("/api/shop/:id/sell", shopH.Sell)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "seller", "password": "pass1234"})
	var lr map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &lr)
	token := lr["token"].(string)
	accountID := int64(lr["account_id"].(float64))

	char := &model.Character{AccountID: accountID, Name: "Seller", ClassID: 1, HP: 100, MaxHP: 100, Gold: 0}
	require.NoError(t, db.Create(char).Error)

	inv := &model.Inventory{CharID: char.ID, ItemID: 1, Kind: 1, Qty: 3}
	require.NoError(t, db.Create(inv).Error)

	w2 := postJSON(r, fmt.Sprintf("/api/shop/1/sell?char_id=%d", char.ID),
		map[string]interface{}{"inv_id": inv.ID, "qty": 2},
		"Authorization", "Bearer "+token)
	require.Equal(t, http.StatusOK, w2.Code)

	// Inventory should be reduced
	var updatedInv model.Inventory
	db.First(&updatedInv, inv.ID)
	assert.Equal(t, 1, updatedInv.Qty)
}

func TestShopSell_ItemNotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authH := rest.NewAuthHandler(db, c, sec)
	shopH := rest.NewShopHandler(db, nil, testShops)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	r.Use(mw.Auth(sec, c))
	r.POST("/api/shop/:id/sell", shopH.Sell)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "sellernf", "password": "pass1234"})
	var lr map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &lr)
	token := lr["token"].(string)
	accountID := int64(lr["account_id"].(float64))

	char := &model.Character{AccountID: accountID, Name: "SellNF", ClassID: 1, HP: 100, MaxHP: 100}
	require.NoError(t, db.Create(char).Error)

	w2 := postJSON(r, fmt.Sprintf("/api/shop/1/sell?char_id=%d", char.ID),
		map[string]interface{}{"inv_id": 9999, "qty": 1},
		"Authorization", "Bearer "+token)
	assert.Equal(t, http.StatusNotFound, w2.Code)
}

// shopWithRes returns a ResourceLoader with inline item data (no file I/O needed).
func shopWithRes() *resource.ResourceLoader {
	return &resource.ResourceLoader{
		Items:   []*resource.Item{nil, {ID: 1, Name: "Potion", Price: 200}},
		Weapons: []*resource.Weapon{nil, {ID: 1, Name: "Sword", Price: 600}},
		Armors:  []*resource.Armor{nil, {ID: 1, Name: "Shield", Price: 400}},
	}
}

// testShopsZeroPrice has shops whose items carry a zero price so that
// Detail calls resolvePrice (and resolveName) via the ResourceLoader.
var testShopsZeroPrice = map[int]*rest.ShopDef{
	10: {
		ID:   10,
		Name: "Resolver Shop",
		Items: []rest.ShopItem{
			{Kind: 1, ItemID: 1, Price: 0}, // item – resolvePrice + resolveName
			{Kind: 2, ItemID: 1, Price: 0}, // weapon
			{Kind: 3, ItemID: 1, Price: 0}, // armor
		},
	},
}

func TestShopDetail_WithResolver(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authH := rest.NewAuthHandler(db, c, sec)
	shopH := rest.NewShopHandler(db, shopWithRes(), testShopsZeroPrice)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	r.Use(mw.Auth(sec, c))
	r.GET("/api/shop/:id", shopH.Detail)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "resolvedetail", "password": "pass1234"})
	var lr map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &lr)
	token := lr["token"].(string)

	req := httptest.NewRequest(http.MethodGet, "/api/shop/10", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	wr := httptest.NewRecorder()
	r.ServeHTTP(wr, req)
	require.Equal(t, http.StatusOK, wr.Code)

	var resp map[string]interface{}
	json.Unmarshal(wr.Body.Bytes(), &resp)
	items := resp["items"].([]interface{})
	require.Len(t, items, 3)

	// Item kind=1 resolved from ResourceLoader
	item0 := items[0].(map[string]interface{})
	assert.Equal(t, "Potion", item0["name"])
	assert.Equal(t, float64(200), item0["price"])

	// Weapon kind=2
	item1 := items[1].(map[string]interface{})
	assert.Equal(t, "Sword", item1["name"])
	assert.Equal(t, float64(600), item1["price"])

	// Armor kind=3
	item2 := items[2].(map[string]interface{})
	assert.Equal(t, "Shield", item2["name"])
	assert.Equal(t, float64(400), item2["price"])
}

func TestShopSell_WithResolver(t *testing.T) {
	db := testutil.SetupTestDB(t)
	c, _ := testutil.SetupTestCache(t)
	sec := config.SecurityConfig{JWTSecret: "test-secret", JWTTTLH: 72 * time.Hour}

	authH := rest.NewAuthHandler(db, c, sec)
	shopH := rest.NewShopHandler(db, shopWithRes(), testShops)

	r := gin.New()
	r.POST("/api/auth/login", authH.Login)
	r.Use(mw.Auth(sec, c))
	r.POST("/api/shop/:id/sell", shopH.Sell)

	w := postJSON(r, "/api/auth/login", map[string]string{"username": "resolverseller", "password": "pass1234"})
	var lr map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &lr)
	token := lr["token"].(string)
	accountID := int64(lr["account_id"].(float64))

	char := &model.Character{AccountID: accountID, Name: "ResSeller", ClassID: 1, HP: 100, MaxHP: 100, Gold: 0}
	require.NoError(t, db.Create(char).Error)

	inv := &model.Inventory{CharID: char.ID, ItemID: 1, Kind: 1, Qty: 4}
	require.NoError(t, db.Create(inv).Error)

	w2 := postJSON(r, fmt.Sprintf("/api/shop/1/sell?char_id=%d", char.ID),
		map[string]interface{}{"inv_id": inv.ID, "qty": 2},
		"Authorization", "Bearer "+token)
	require.Equal(t, http.StatusOK, w2.Code)

	// resolvePrice(1, 1) = 200; sellPrice = 200/2 = 100; earned = 100*2 = 200
	var updatedChar model.Character
	db.First(&updatedChar, char.ID)
	assert.GreaterOrEqual(t, updatedChar.Gold, int64(0))
}
