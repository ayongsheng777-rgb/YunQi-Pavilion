package yunqi

import (
	"encoding/json"
	"flatnasgo-backend/models"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// 模块 3：MCP 服务端
// 完整移植 ref/mcp_server.go 的协议层（JSON-RPC 2.0 over HTTP），
// 数据层改写为 FlatNas 的 models/utils/config（不引用 sun-panel 的包）。
// 鉴权：与 OpenAPI 同一个 token（Authorization: Bearer 或 token 头）。
// 路由：POST /mcp
// MCP 请求里没有 JWT 用户，书签/签到/AI 工具统一用默认 admin 用户上下文。
// ---------------------------------------------------------------------------

var mcpStartTime = time.Now()

// JSON-RPC 2.0 结构（照搬 ref/mcp_server.go）
type mcpRpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type mcpRpcResponse struct {
	JSONRPC string       `json:"jsonrpc"`
	ID      interface{}  `json:"id"`
	Result  interface{}  `json:"result,omitempty"`
	Error   *mcpRpcError `json:"error,omitempty"`
}

type mcpRpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

type mcpToolsListResult struct {
	Tools []mcpTool `json:"tools"`
}

type mcpCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type mcpCallResult struct {
	Content []mcpContentItem `json:"content"`
	IsError bool             `json:"isError,omitempty"`
}

type mcpContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func strProp(desc string) map[string]interface{} {
	return map[string]interface{}{"type": "string", "description": desc}
}
func intProp(desc string) map[string]interface{} {
	return map[string]interface{}{"type": "integer", "description": desc}
}

// 工具集：每个都有中文 description 和完整 inputSchema
var mcpAvailableTools = []mcpTool{
	{
		Name:        "search_bookmarks",
		Description: "按关键词搜索用户的书签/网址导航。搜索范围：标题、网址、描述。返回命中的书签标题与网址。",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": strProp("搜索关键词（必填）"),
				"limit": intProp("最多返回条数（默认20，最多100）"),
			},
			"required": []string{"query"},
		},
	},
	{
		Name:        "list_categories",
		Description: "列出用户导航里的全部书签分类（分组），含每个分类的书签数量。",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	},
	{
		Name:        "list_bookmarks",
		Description: "列出指定分类下的书签。先用 list_categories 查到分类 ID。",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"category_id": strProp("分类/分组 ID（必填）"),
				"limit":       intProp("最多返回条数（默认50，最多200）"),
			},
			"required": []string{"category_id"},
		},
	},
	{
		Name:        "get_bookmark",
		Description: "按书签 ID 获取单个书签的完整信息（含全部网址与描述）。",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id": strProp("书签 ID（必填）"),
			},
			"required": []string{"id"},
		},
	},
	{
		Name:        "create_bookmark",
		Description: "新建一个书签。标题和网址必填；分类 ID 可选（不填则放到第一个分类）；描述可选。",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"title":       strProp("书签标题（必填）"),
				"url":         strProp("书签网址（必填）"),
				"category_id": strProp("分类/分组 ID（可选，默认第一个分类）"),
				"description": strProp("描述（可选）"),
			},
			"required": []string{"title", "url"},
		},
	},
	{
		Name:        "update_bookmark",
		Description: "按 ID 更新一个书签。只改传了的字段，没传的保持原样。",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id":          strProp("书签 ID（必填）"),
				"title":       strProp("新标题（可选）"),
				"url":         strProp("新网址（可选）"),
				"description": strProp("新描述（可选）"),
				"category_id": strProp("移动到新的分类/分组 ID（可选）"),
			},
			"required": []string{"id"},
		},
	},
	{
		Name:        "delete_bookmark",
		Description: "按 ID 删除一个书签。",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id": strProp("书签 ID（必填）"),
			},
			"required": []string{"id"},
		},
	},
	{
		Name:        "checkin_sites",
		Description: "列出签到提醒里的全部站点（含是否启用）。",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	},
	{
		Name:        "checkin_today_status",
		Description: "查询今天各签到站点的签到状态（已签/未签）。",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	},
	{
		Name:        "checkin_mark_done",
		Description: "把某个签到站点标记为今天已签到。",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"site_id": strProp("站点 ID（必填）"),
			},
			"required": []string{"site_id"},
		},
	},
	{
		Name:        "ai_ask",
		Description: "向云栖阁 AI 助手提问。AI 能查你导航里的书签，并用大白话中文回答。",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"message": strProp("你的问题（必填）"),
			},
			"required": []string{"message"},
		},
	},
	{
		Name:        "system_status",
		Description: "查看云栖阁服务状态：版本号、Go 版本、已运行时间。",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	},
}

// MCPHandle 是唯一的 JSON-RPC 入口：POST /mcp
func MCPHandle(c *gin.Context) {
	var req mcpRpcRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, mcpRpcResponse{
			JSONRPC: "2.0",
			Error:   &mcpRpcError{Code: -32700, Message: "请求格式不对：不是合法 JSON"},
		})
		return
	}

	resp := mcpRpcResponse{JSONRPC: "2.0", ID: req.ID}

	switch req.Method {
	case "initialize":
		resp.Result = map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name":    "yunqi-pavilion-mcp",
				"version": "1.0.0",
			},
		}
	case "notifications/initialized":
		resp.Result = map[string]interface{}{}
	case "tools/list":
		resp.Result = mcpToolsListResult{Tools: mcpAvailableTools}
	case "tools/call":
		var params mcpCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &mcpRpcError{Code: -32602, Message: "参数格式不对"}
			break
		}
		result, isErr := mcpCallTool(params.Name, params.Arguments)
		resp.Result = mcpCallResult{
			Content: []mcpContentItem{{Type: "text", Text: result}},
			IsError: isErr,
		}
	case "ping":
		resp.Result = map[string]interface{}{}
	default:
		resp.Error = &mcpRpcError{Code: -32601, Message: "不支持的方法：" + req.Method}
	}

	c.JSON(http.StatusOK, resp)
}

func mcpStrArg(args map[string]interface{}, key string) string {
	if v, ok := args[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func mcpIntArg(args map[string]interface{}, key string, def int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return def
}

// mcpCallTool 分发工具调用（统一用 admin 用户上下文）
func mcpCallTool(name string, args map[string]interface{}) (string, bool) {
	const user = "admin"
	if args == nil {
		args = map[string]interface{}{}
	}
	switch name {
	case "search_bookmarks":
		query := mcpStrArg(args, "query")
		limit := mcpIntArg(args, "limit", 20)
		if limit > 100 {
			limit = 100
		}
		return mcpSearchBookmarks(user, query, limit)
	case "list_categories":
		return mcpListCategories(user)
	case "list_bookmarks":
		catID := mcpStrArg(args, "category_id")
		limit := mcpIntArg(args, "limit", 50)
		if limit > 200 {
			limit = 200
		}
		return mcpListBookmarks(user, catID, limit)
	case "get_bookmark":
		return mcpGetBookmark(user, mcpStrArg(args, "id"))
	case "create_bookmark":
		return mcpCreateBookmark(user, args)
	case "update_bookmark":
		return mcpUpdateBookmark(user, args)
	case "delete_bookmark":
		return mcpDeleteBookmark(user, mcpStrArg(args, "id"))
	case "checkin_sites":
		return mcpCheckinSites(user)
	case "checkin_today_status":
		return mcpCheckinTodayStatus(user)
	case "checkin_mark_done":
		return mcpCheckinMarkDone(user, mcpStrArg(args, "site_id"))
	case "ai_ask":
		return mcpAIAsk(user, mcpStrArg(args, "message"))
	case "system_status":
		return mcpSystemStatus()
	default:
		return "没有这个工具：" + name, true
	}
}

func mcpLoadGroups(user string) ([]models.Group, error) {
	return loadBookmarkGroups(UserDataFile(user))
}

func mcpFormatBookmarkLine(g models.Group, it models.Item) string {
	desc := it.Description1
	if len([]rune(desc)) > 60 {
		desc = truncateRunes(desc, 60) + "…"
	}
	line := "[" + it.ID + "] " + it.Title + " - " + it.Url + "（分类：" + g.Title + "）"
	if desc != "" {
		line += " | " + desc
	}
	return line
}

func mcpSearchBookmarks(user, query string, limit int) (string, bool) {
	if query == "" {
		return "请传入搜索关键词 query", true
	}
	groups, err := mcpLoadGroups(user)
	if err != nil {
		return "读取书签失败", true
	}
	q := strings.ToLower(query)
	var lines []string
	for _, g := range groups {
		for _, it := range g.Items {
			hay := strings.ToLower(it.Title + " " + it.Url + " " + it.Description1)
			if strings.Contains(hay, q) {
				lines = append(lines, mcpFormatBookmarkLine(g, it))
				if len(lines) >= limit {
					break
				}
			}
		}
		if len(lines) >= limit {
			break
		}
	}
	if len(lines) == 0 {
		return "没找到和「" + query + "」相关的书签", false
	}
	return "找到 " + fmt.Sprint(len(lines)) + " 个书签：\n" + strings.Join(lines, "\n"), false
}

func mcpListCategories(user string) (string, bool) {
	groups, err := mcpLoadGroups(user)
	if err != nil {
		return "读取分类失败", true
	}
	if len(groups) == 0 {
		return "还没有任何分类", false
	}
	var sb strings.Builder
	sb.WriteString("分类列表：\n")
	for _, g := range groups {
		sb.WriteString("[" + g.ID + "] " + g.Title + "（" + fmt.Sprint(len(g.Items)) + " 个书签）\n")
	}
	return strings.TrimRight(sb.String(), "\n"), false
}

func mcpListBookmarks(user, categoryID string, limit int) (string, bool) {
	if categoryID == "" {
		return "请传入分类 ID category_id", true
	}
	groups, err := mcpLoadGroups(user)
	if err != nil {
		return "读取书签失败", true
	}
	gi := findGroupByID(groups, categoryID)
	if gi < 0 {
		return "找不到这个分类", true
	}
	g := groups[gi]
	if len(g.Items) == 0 {
		return "分类「" + g.Title + "」里还没有书签", false
	}
	var lines []string
	for _, it := range g.Items {
		lines = append(lines, mcpFormatBookmarkLine(g, it))
		if len(lines) >= limit {
			break
		}
	}
	return "分类「" + g.Title + "」的书签：\n" + strings.Join(lines, "\n"), false
}

func mcpGetBookmark(user, id string) (string, bool) {
	if id == "" {
		return "请传入书签 ID", true
	}
	groups, err := mcpLoadGroups(user)
	if err != nil {
		return "读取书签失败", true
	}
	gi, ii := findItemAcrossGroups(groups, id)
	if gi < 0 {
		return "找不到这个书签", true
	}
	g := groups[gi]
	it := g.Items[ii]
	var sb strings.Builder
	sb.WriteString("ID：" + it.ID + "\n")
	sb.WriteString("标题：" + it.Title + "\n")
	sb.WriteString("网址：" + it.Url + "\n")
	if it.Description1 != "" {
		sb.WriteString("描述：" + it.Description1 + "\n")
	}
	sb.WriteString("分类：" + g.Title + "（" + g.ID + "）")
	return sb.String(), false
}

func mcpCreateBookmark(user string, args map[string]interface{}) (string, bool) {
	title := mcpStrArg(args, "title")
	url := mcpStrArg(args, "url")
	if title == "" {
		return "标题不能为空", true
	}
	if url == "" {
		return "网址不能为空", true
	}
	description := mcpStrArg(args, "description")
	categoryID := mcpStrArg(args, "category_id")

	userFile := UserDataFile(user)
	groups, err := loadBookmarkGroups(userFile)
	if err != nil {
		return "读取书签失败", true
	}
	// 去重：网址已存在则直接提示
	if dupGroup, dup := findDuplicateBookmark(groups, url); dup != nil {
		return "该网址已收藏过（分类：" + dupGroup + "，标题：" + dup.Title + "），未重复创建", true
	}
	gi := -1
	if categoryID != "" {
		gi = findGroupByID(groups, categoryID)
		if gi < 0 {
			return "找不到指定的分类", true
		}
	} else {
		if len(groups) == 0 {
			groups = append(groups, models.Group{ID: newUUID(), Title: "未分类", Items: []models.Item{}})
		}
		gi = 0
	}
	item := models.Item{
		ID:           newUUID(),
		Title:        title,
		Url:          url,
		Description1: description,
	}
	groups[gi].Items = append(groups[gi].Items, item)
	if err := saveBookmarkGroups(userFile, groups); err != nil {
		return "保存失败", true
	}
	return "书签已创建，ID：" + item.ID + "，分类：" + groups[gi].Title, false
}

func mcpUpdateBookmark(user string, args map[string]interface{}) (string, bool) {
	id := mcpStrArg(args, "id")
	if id == "" {
		return "请传入书签 ID", true
	}
	userFile := UserDataFile(user)
	groups, err := loadBookmarkGroups(userFile)
	if err != nil {
		return "读取书签失败", true
	}
	gi, ii := findItemAcrossGroups(groups, id)
	if gi < 0 {
		return "找不到这个书签", true
	}
	changed := false
	if v := mcpStrArg(args, "title"); v != "" {
		groups[gi].Items[ii].Title = v
		changed = true
	}
	if v := mcpStrArg(args, "url"); v != "" {
		groups[gi].Items[ii].Url = v
		changed = true
	}
	// description 允许清空：只要传了就按传的值写
	if v, ok := args["description"].(string); ok {
		groups[gi].Items[ii].Description1 = strings.TrimSpace(v)
		changed = true
	}
	if newCat := mcpStrArg(args, "category_id"); newCat != "" {
		ngi := findGroupByID(groups, newCat)
		if ngi < 0 {
			return "找不到目标分类", true
		}
		if ngi != gi {
			item := groups[gi].Items[ii]
			groups[gi].Items = append(groups[gi].Items[:ii], groups[gi].Items[ii+1:]...)
			groups[ngi].Items = append(groups[ngi].Items, item)
		}
		changed = true
	}
	if !changed {
		return "没有要改的字段", true
	}
	if err := saveBookmarkGroups(userFile, groups); err != nil {
		return "保存失败", true
	}
	return "书签 " + id + " 已更新", false
}

func mcpDeleteBookmark(user, id string) (string, bool) {
	if id == "" {
		return "请传入书签 ID", true
	}
	userFile := UserDataFile(user)
	groups, err := loadBookmarkGroups(userFile)
	if err != nil {
		return "读取书签失败", true
	}
	gi, ii := findItemAcrossGroups(groups, id)
	if gi < 0 {
		return "找不到这个书签", true
	}
	title := groups[gi].Items[ii].Title
	groups[gi].Items = append(groups[gi].Items[:ii], groups[gi].Items[ii+1:]...)
	if err := saveBookmarkGroups(userFile, groups); err != nil {
		return "保存失败", true
	}
	return "已删除书签：" + title, false
}

func mcpCheckinSites(user string) (string, bool) {
	checkinMu.Lock()
	st := loadCheckinStore()
	checkinMu.Unlock()
	sites := st.Sites[user]
	if len(sites) == 0 {
		return "签到提醒里还没有站点", false
	}
	var sb strings.Builder
	sb.WriteString("签到站点：\n")
	for _, s := range sites {
		state := "启用"
		if !s.Enabled {
			state = "停用"
		}
		sb.WriteString("[" + s.ID + "] " + s.Name + " - " + s.URL + "（" + state + "）\n")
	}
	return strings.TrimRight(sb.String(), "\n"), false
}

func mcpCheckinTodayStatus(user string) (string, bool) {
	checkinMu.Lock()
	st := loadCheckinStore()
	rc := loadCheckinRecords()
	checkinMu.Unlock()
	date := todayStr()
	doneIDs := rc.Dates[date][user]
	sites := st.Sites[user]
	if len(sites) == 0 {
		return "签到提醒里还没有站点", false
	}
	var sb strings.Builder
	sb.WriteString(date + " 签到状态：\n")
	for _, s := range sites {
		mark := "未签"
		if checkinHasDone(doneIDs, s.ID) {
			mark = "已签"
		}
		sb.WriteString("[" + mark + "] " + s.Name + "\n")
	}
	return strings.TrimRight(sb.String(), "\n"), false
}

func mcpCheckinMarkDone(user, siteID string) (string, bool) {
	if siteID == "" {
		return "请传入站点 ID site_id", true
	}
	checkinMu.Lock()
	defer checkinMu.Unlock()
	st := loadCheckinStore()
	found := false
	var name string
	for _, s := range st.Sites[user] {
		if s.ID == siteID {
			found = true
			name = s.Name
			break
		}
	}
	if !found {
		return "找不到这个站点", true
	}
	rc := loadCheckinRecords()
	markCheckinDoneLocked(rc, user, siteID)
	if err := saveCheckinRecords(rc); err != nil {
		return "记录签到失败", true
	}
	return "「" + name + "」已标记为今日已签到", false
}

func mcpAIAsk(user, message string) (string, bool) {
	if strings.TrimSpace(message) == "" {
		return "请传入问题 message", true
	}
	reply, err := AIAsk(user, message)
	if err != nil {
		return "AI 出错了：" + err.Error(), true
	}
	return reply, false
}

func mcpSystemStatus() (string, bool) {
	uptime := time.Since(mcpStartTime).Truncate(time.Second)
	return "云栖阁 YunQi Pavilion\n版本：1.0.0（基于 FlatNas）\nGo：" + runtime.Version() +
		"\n已运行：" + uptime.String(), false
}
