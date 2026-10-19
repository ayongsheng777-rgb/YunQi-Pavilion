package yunqi

import (
	"crypto/subtle"
	"flatnasgo-backend/models"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// 模块 2：OpenAPI 收藏接口（给未来 Chrome 插件用）
// token 鉴权：Authorization: Bearer <token> 或 ?token= ；
// token 存 config.DataDir/yunqi/api_token.txt，首次启动自动生成 32 字节随机 hex。
// 书签写入 FlatNas 数据：single 模式用 admin 的 data.json（走 UserDataFile 定位）。
// ---------------------------------------------------------------------------

var (
	apiTokenMu  sync.Mutex
	apiTokenVal string
)

func apiTokenFile() string { return filepath.Join(YunqiDir(), "api_token.txt") }

// EnsureAPIToken 确保 token 文件存在，不存在则生成（首次启动调用）。
func EnsureAPIToken() string {
	apiTokenMu.Lock()
	defer apiTokenMu.Unlock()
	if apiTokenVal != "" {
		return apiTokenVal
	}
	data, err := os.ReadFile(apiTokenFile())
	if err == nil {
		if t := strings.TrimSpace(string(data)); t != "" {
			apiTokenVal = t
			return apiTokenVal
		}
	}
	tok, err := randomHex(32)
	if err != nil {
		return ""
	}
	// 0600：token 是密钥，最小权限
	_ = os.WriteFile(apiTokenFile(), []byte(tok), 0600)
	apiTokenVal = tok
	return apiTokenVal
}

// RegenerateAPIToken 重新生成 token（会覆盖旧文件）。
func RegenerateAPIToken() (string, error) {
	apiTokenMu.Lock()
	defer apiTokenMu.Unlock()
	tok, err := randomHex(32)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(apiTokenFile(), []byte(tok), 0600); err != nil {
		return "", err
	}
	apiTokenVal = tok
	return tok, nil
}

func getAPIToken() string {
	if t := EnsureAPIToken(); t != "" {
		return t
	}
	return apiTokenVal
}

// extractOpenAPIToken 从 Bearer 头、token 头或 ?token= 取 token。
func extractOpenAPIToken(c *gin.Context) string {
	if t := strings.TrimSpace(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")); t != "" {
		return t
	}
	if t := strings.TrimSpace(c.GetHeader("token")); t != "" {
		return t
	}
	return strings.TrimSpace(c.Query("token"))
}

func checkOpenAPIToken(c *gin.Context) bool {
	got := extractOpenAPIToken(c)
	want := getAPIToken()
	if got == "" || want == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// OpenAPIAuthMiddleware OpenAPI/MCP 共用的 token 鉴权中间件。
func OpenAPIAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !checkOpenAPIToken(c) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token 无效或缺失"})
			return
		}
		c.Next()
	}
}

type openapiBookmarkReq struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

// POST /openapi/v1/bookmark/create
func OpenAPICreateBookmark(c *gin.Context) {
	var req openapiBookmarkReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式不对"})
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	req.URL = strings.TrimSpace(req.URL)
	req.Category = strings.TrimSpace(req.Category)
	req.Description = strings.TrimSpace(req.Description)
	if req.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "标题不能为空"})
		return
	}
	if req.URL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "网址不能为空"})
		return
	}
	if req.Category == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "分类不能为空"})
		return
	}

	// 去重：网址已存在则直接提示，不重复创建
	userFile := UserDataFile("admin")
	if groups, err := loadBookmarkGroups(userFile); err == nil {
		if dupGroup, dup := findDuplicateBookmark(groups, req.URL); dup != nil {
			c.JSON(http.StatusConflict, gin.H{
				"error":    "该网址已收藏过",
				"duplicate": true,
				"category": dupGroup,
				"title":    dup.Title,
				"id":       dup.ID,
			})
			return
		}
	}

	id, err := addBookmarkToCategory("admin", req.Category, req.Title, req.URL, req.Description)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败，请稍后再试"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "id": id})
}

// addBookmarkToCategory 按分类标题找 Group（没有就新建），追加一条书签，返回新书签 id。
func addBookmarkToCategory(username, category, title, url, description string) (string, error) {
	userFile := UserDataFile(username)
	groups, err := loadBookmarkGroups(userFile)
	if err != nil {
		return "", err
	}
	gi := findGroupByTitle(groups, category)
	if gi < 0 {
		groups = append(groups, models.Group{
			ID:    newUUID(),
			Title: category,
			Items: []models.Item{},
		})
		gi = len(groups) - 1
	}
	item := models.Item{
		ID:           newUUID(),
		Title:        title,
		Url:          url,
		Description1: description,
	}
	groups[gi].Items = append(groups[gi].Items, item)
	if err := saveBookmarkGroups(userFile, groups); err != nil {
		return "", err
	}
	return item.ID, nil
}

// GET /openapi/v1/categories
func OpenAPIListCategories(c *gin.Context) {
	groups, err := loadBookmarkGroups(UserDataFile("admin"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取分类失败"})
		return
	}
	out := []gin.H{}
	for _, g := range groups {
		out = append(out, gin.H{"id": g.ID, "title": g.Title, "count": len(g.Items)})
	}
	c.JSON(http.StatusOK, gin.H{"categories": out})
}

// GET /openapi/v1/version
func OpenAPIVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"name":     "云栖阁 YunQi Pavilion",
		"version":  "1.0.0",
		"based_on": "FlatNas",
	})
}

// GET /api/yunqi/token（JWT 鉴权）查看当前 token
func YunqiGetToken(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"token": getAPIToken()})
}

type yunqiTokenRegenReq struct {
	Regenerate bool `json:"regenerate"`
}

// POST /api/yunqi/token（JWT 鉴权）{"regenerate":true} 换新 token
func YunqiRegenerateToken(c *gin.Context) {
	var req yunqiTokenRegenReq
	_ = c.ShouldBindJSON(&req) // 空 body 也允许：直接返回当前 token
	if !req.Regenerate {
		c.JSON(http.StatusOK, gin.H{"token": getAPIToken()})
		return
	}
	tok, err := RegenerateAPIToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成新 token 失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": tok})
}
