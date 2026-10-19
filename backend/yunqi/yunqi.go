// Package yunqi 实现「云栖阁 YunQi Pavilion」的后端扩展模块：
//
//	checkin  - 网站签到提醒
//	openapi  - 给浏览器插件用的收藏接口（含 token 鉴权）
//	mcp      - MCP 服务端（JSON-RPC 2.0 over HTTP）
//	ai       - AI 聊天（OpenAI 兼容协议 + 两轮 function-calling）
//	otp      - TOTP 双因素 + 30 天可信设备
//
// 数据文件统一放在 config.DataDir/yunqi/ 下。
// 对外品牌名「云栖阁 YunQi Pavilion」只出现在用户可见文案里，module 名保持 flatnasgo-backend 不动。
package yunqi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flatnasgo-backend/config"
	"flatnasgo-backend/models"
	"flatnasgo-backend/utils"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// 目录与通用小工具
// ---------------------------------------------------------------------------

var yunqiDirOnce sync.Once

// YunqiDir 返回云栖阁扩展数据目录（config.DataDir/yunqi），不存在则创建。
func YunqiDir() string {
	dir := filepath.Join(config.DataDir, "yunqi")
	yunqiDirOnce.Do(func() {
		_ = os.MkdirAll(dir, 0755)
	})
	// 兜底：万一 Once 之后目录被删，每次都尝试创建（MkdirAll 已存在时无开销）
	_ = os.MkdirAll(dir, 0755)
	return dir
}

// Init 首次启动时做一次性初始化：建目录、生成 OpenAPI token。
// 在 main.go 的 config.Init() 之后调用。
func Init() {
	_ = YunqiDir()
	EnsureAPIToken()
}

// truncateRunes 按 rune 安全截断字符串，避免把中文/emoji 砍半。
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

func newUUID() string { return uuid.NewString() }

func randomHex(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ---------------------------------------------------------------------------
// 用户数据文件定位（与 handlers/data.go、handlers/auth.go 的逻辑保持一致）
// ---------------------------------------------------------------------------

// systemAuthMode 读取系统配置里的 authMode（single/multi），读不到默认 single。
func systemAuthMode() string {
	var cfg models.SystemConfig
	if err := utils.ReadJSON(config.SystemConfigFile, &cfg); err != nil {
		return "single"
	}
	if strings.TrimSpace(cfg.AuthMode) == "" {
		return "single"
	}
	return cfg.AuthMode
}

// UserDataFile 返回某用户名的书签数据文件路径。
// single 模式的 admin 用 config.DataDir/data.json，其余走 config.UsersDir/<username>.json。
func UserDataFile(username string) string {
	if username == "" {
		username = "admin"
	}
	if username == "admin" && systemAuthMode() == "single" {
		return filepath.Join(config.DataDir, "data.json")
	}
	return filepath.Join(config.UsersDir, username+".json")
}

// ---------------------------------------------------------------------------
// 书签（Group/Item）读写：用 map 读回写，避免 models.User 丢掉 version/username
// 等 data.json 顶层字段；写完按 data.go 的缓存失效方式处理
// （data.go 的 getDataCache 以文件 modtime 校验，utils.WriteJSON 原子写会更新
// modtime，天然失效；这里再把 version +1，让前端的版本比对也保持一致）。
// ---------------------------------------------------------------------------

var bookmarkMu sync.Mutex

func loadBookmarkGroups(userFile string) ([]models.Group, error) {
	var data map[string]interface{}
	if err := utils.ReadJSON(userFile, &data); err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(data["groups"])
	var groups []models.Group
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &groups); err != nil {
			return nil, err
		}
	}
	if groups == nil {
		groups = []models.Group{}
	}
	return groups, nil
}

func saveBookmarkGroups(userFile string, groups []models.Group) error {
	bookmarkMu.Lock()
	defer bookmarkMu.Unlock()

	var data map[string]interface{}
	if err := utils.ReadJSON(userFile, &data); err != nil {
		return err
	}
	if data == nil {
		data = map[string]interface{}{}
	}
	raw, err := json.Marshal(groups)
	if err != nil {
		return err
	}
	var arr []interface{}
	if err := json.Unmarshal(raw, &arr); err != nil {
		return err
	}
	data["groups"] = arr
	data["version"] = normalizeVersionValue(data["version"]) + 1
	return utils.WriteJSON(userFile, data)
}

func normalizeVersionValue(v interface{}) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case float32:
		return int64(t)
	case int:
		return int64(t)
	case int64:
		return t
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return n
		}
	}
	return 0
}

// findGroupByTitle 按标题精确查找分组，返回下标（-1 表示没找到）。
func findGroupByTitle(groups []models.Group, title string) int {
	for i := range groups {
		if groups[i].Title == title {
			return i
		}
	}
	return -1
}

// findGroupByID 按 id 查找分组，返回下标。
func findGroupByID(groups []models.Group, id string) int {
	for i := range groups {
		if groups[i].ID == id {
			return i
		}
	}
	return -1
}

// findItemAcrossGroups 在全部分组里找书签，返回 group 下标、item 下标。
func findItemAcrossGroups(groups []models.Group, itemID string) (gi, ii int) {
	for gi = range groups {
		for ii = range groups[gi].Items {
			if groups[gi].Items[ii].ID == itemID {
				return gi, ii
			}
		}
	}
	return -1, -1
}

// ---------------------------------------------------------------------------
// JWT 签发（与 handlers.Login 原逻辑一致：HS256，30 天有效期）
// ---------------------------------------------------------------------------

// IssueUserToken 签发普通登录 JWT，供 handlers.Login 与 OTP 二次验证后复用。
func IssueUserToken(username string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": username,
		"exp":      time.Now().Add(time.Hour * 24 * 30).Unix(),
	})
	return token.SignedString([]byte(config.GetSecretKeyString()))
}

// ParseUsername 从请求能拿到的 JWT 里解析用户名（忽略 otp_pending 的临时 token）。
func ParseUsername(tokenString string) (string, bool) {
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		return []byte(config.GetSecretKeyString()), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || token == nil || !token.Valid {
		return "", false
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", false
	}
	if pending, _ := claims["otp_pending"].(bool); pending {
		return "", false
	}
	username, _ := claims["username"].(string)
	if username == "" {
		return "", false
	}
	return username, true
}

// bearerOrQueryToken 从 Authorization: Bearer 头或 ?token= 取 token（给 /api/auth/me 这类用）。
func bearerOrQueryToken(authHeader, queryToken string) string {
	if s := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer ")); s != "" {
		return s
	}
	return strings.TrimSpace(queryToken)
}
