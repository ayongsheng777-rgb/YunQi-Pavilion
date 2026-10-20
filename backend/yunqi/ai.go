package yunqi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flatnasgo-backend/utils"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// 模块 4：AI 功能（移植 sun-panel service/lib/ai 精简版）
// 配置存 config.DataDir/yunqi/ai.json。
// 调 OpenAI 兼容的 /chat/completions；system prompt 强制大白话中文；
// 两轮 function-calling：第一轮拿 tool_calls，第二轮把本地书签结果喂回去再答。
// ---------------------------------------------------------------------------

const (
	defaultAIBaseURL = "https://api.deepseek.com/v1"
	defaultAIModel   = "deepseek-chat"
	defaultAIProxy   = "http://192.168.57.228:7890" // J4125 sing-box mixed 代理
	aiTimeout        = 60 * time.Second
)

// AI 平台预设：OpenAI 兼容接口
type aiProviderPreset struct {
	Name     string `json:"name"`
	BaseURL  string `json:"base_url"`
	Model    string `json:"model"`
	NeedKey  bool   `json:"need_key"`
	KeyURL   string `json:"key_url"`   // 申请 key 的网址
	Homepage string `json:"homepage"`  // 官网
}

var aiProviderPresets = []aiProviderPreset{
	{Name: "DeepSeek 开放平台", BaseURL: "https://api.deepseek.com/v1", Model: "deepseek-chat", NeedKey: true, KeyURL: "https://platform.deepseek.com/api_keys", Homepage: "https://platform.deepseek.com"},
	{Name: "Kimi（月之暗面）", BaseURL: "https://api.moonshot.cn/v1", Model: "kimi-k2", NeedKey: true, KeyURL: "https://platform.moonshot.cn/console/api-keys", Homepage: "https://platform.moonshot.cn"},
	{Name: "阿里云百炼", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", Model: "qwen-plus", NeedKey: true, KeyURL: "https://bailian.console.aliyun.com/?apiKey=1#/api-key", Homepage: "https://bailian.console.aliyun.com"},
	{Name: "硅基流动", BaseURL: "https://api.siliconflow.cn/v1", Model: "Qwen/Qwen2.5-7B-Instruct", NeedKey: true, KeyURL: "https://cloud.siliconflow.cn/account/ak", Homepage: "https://cloud.siliconflow.cn"},
	{Name: "魔搭 ModelScope", BaseURL: "https://api-inference.modelscope.cn/v1", Model: "Qwen/Qwen2.5-7B-Instruct", NeedKey: true, KeyURL: "https://modelscope.cn/my/myaccesstoken", Homepage: "https://modelscope.cn"},
	{Name: "零一万物", BaseURL: "https://api.lingyiwanwu.com/v1", Model: "yi-lightning", NeedKey: true, KeyURL: "https://platform.lingyiwanwu.com", Homepage: "https://platform.lingyiwanwu.com"},
	{Name: "OpenRouter", BaseURL: "https://openrouter.ai/api/v1", Model: "deepseek/deepseek-chat", NeedKey: true, KeyURL: "https://openrouter.ai/keys", Homepage: "https://openrouter.ai"},
	{Name: "GroqCloud", BaseURL: "https://api.groq.com/openai/v1", Model: "llama-3.3-70b-versatile", NeedKey: true, KeyURL: "https://console.groq.com/keys", Homepage: "https://console.groq.com"},
	{Name: "Google AI Studio", BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai", Model: "gemini-2.0-flash", NeedKey: true, KeyURL: "https://aistudio.google.com/apikey", Homepage: "https://aistudio.google.com"},
	{Name: "Cloudflare Workers AI", BaseURL: "https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/v1", Model: "@cf/meta/llama-3.1-8b-instruct", NeedKey: true, KeyURL: "https://dash.cloudflare.com/profile/api-tokens", Homepage: "https://developers.cloudflare.com/workers-ai"},
	{Name: "自定义（OpenAI 兼容）", BaseURL: "", Model: "", NeedKey: true, KeyURL: "", Homepage: ""},
}

type aiConfig struct {
	Provider     string `json:"provider"`      // 平台名称，自定义填"自定义"
	BaseURL      string `json:"base_url"`
	APIKey       string `json:"api_key"`
	Model        string `json:"model"`
	ProxyEnabled bool   `json:"proxy_enabled"` // 国际模型走代理
	ProxyURL     string `json:"proxy_url"`
}

var aiConfigMu sync.Mutex

func aiConfigFile() string { return filepath.Join(YunqiDir(), "ai.json") }

func loadAIConfig() aiConfig {
	aiConfigMu.Lock()
	defer aiConfigMu.Unlock()
	cfg := aiConfig{Provider: "DeepSeek 开放平台", BaseURL: defaultAIBaseURL, Model: defaultAIModel, ProxyURL: defaultAIProxy}
	_ = utils.ReadJSON(aiConfigFile(), &cfg)
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = defaultAIBaseURL
	}
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = defaultAIModel
	}
	if strings.TrimSpace(cfg.ProxyURL) == "" {
		cfg.ProxyURL = defaultAIProxy
	}
	return cfg
}

func saveAIConfig(cfg aiConfig) error {
	aiConfigMu.Lock()
	defer aiConfigMu.Unlock()
	return utils.WriteJSON(aiConfigFile(), cfg)
}

// GET /api/ai/config（JWT）-> {"base_url","model","has_key"}
func AIGetConfig(c *gin.Context) {
	cfg := loadAIConfig()
	c.JSON(http.StatusOK, gin.H{
		"provider":      cfg.Provider,
		"base_url":      cfg.BaseURL,
		"model":         cfg.Model,
		"has_key":       strings.TrimSpace(cfg.APIKey) != "",
		"proxy_enabled": cfg.ProxyEnabled,
		"proxy_url":     cfg.ProxyURL,
		"presets":       aiProviderPresets,
	})
}

type aiConfigReq struct {
	Provider     string `json:"provider"`
	BaseURL      string `json:"base_url"`
	APIKey       string `json:"api_key"`
	Model        string `json:"model"`
	ProxyEnabled *bool  `json:"proxy_enabled"`
	ProxyURL     string `json:"proxy_url"`
}

// POST /api/ai/config（JWT）{"provider","base_url","api_key","model","proxy_enabled","proxy_url"}；api_key 留空表示保留原值。
func AISetConfig(c *gin.Context) {
	var req aiConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式不对"})
		return
	}
	cfg := loadAIConfig()
	if strings.TrimSpace(req.Provider) != "" {
		cfg.Provider = strings.TrimSpace(req.Provider)
	}
	if strings.TrimSpace(req.BaseURL) != "" {
		cfg.BaseURL = strings.TrimSpace(req.BaseURL)
	}
	if strings.TrimSpace(req.Model) != "" {
		cfg.Model = strings.TrimSpace(req.Model)
	}
	// 前端留空 = 保留原密钥（密钥只存服务端，永不回显）
	if strings.TrimSpace(req.APIKey) != "" {
		cfg.APIKey = strings.TrimSpace(req.APIKey)
	}
	if req.ProxyEnabled != nil {
		cfg.ProxyEnabled = *req.ProxyEnabled
	}
	if strings.TrimSpace(req.ProxyURL) != "" {
		cfg.ProxyURL = strings.TrimSpace(req.ProxyURL)
	}
	if err := saveAIConfig(cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败，请稍后再试"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ---------------------------------------------------------------------------
// OpenAI 兼容聊天（含 function-calling 两轮）
// ---------------------------------------------------------------------------

type aiChatMessage struct {
	Role      string        `json:"role"`
	Content   string        `json:"content,omitempty"`
	ToolCalls []aiToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
	Name      string        `json:"name,omitempty"`
}

type aiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type aiToolDef struct {
	Type     string `json:"type"`
	Function struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		Parameters  map[string]interface{} `json:"parameters"`
	} `json:"function"`
}

func aiFunctionTools() []aiToolDef {
	mk := func(name, desc string, props map[string]interface{}, required []string) aiToolDef {
		t := aiToolDef{Type: "function"}
		t.Function.Name = name
		t.Function.Description = desc
		t.Function.Parameters = map[string]interface{}{
			"type":       "object",
			"properties": props,
			"required":   required,
		}
		return t
	}
	strProp := func(desc string) map[string]interface{} {
		return map[string]interface{}{"type": "string", "description": desc}
	}
	return []aiToolDef{
		mk("search_bookmarks", "在用户的网址导航里按关键词搜索书签，返回标题和网址。当用户问某个网站、找网址时用它。",
			map[string]interface{}{"query": strProp("搜索关键词")}, []string{"query"}),
		mk("list_categories", "列出用户导航里的全部分类/分组及书签数量。当用户问有哪些分类、导航里有什么时用它。",
			map[string]interface{}{}, nil),
		mk("list_bookmarks", "列出指定分类下的书签。",
			map[string]interface{}{
				"category_id": strProp("分类 ID（可用分类名称，系统会自动匹配）"),
				"limit":       map[string]interface{}{"type": "integer", "description": "最多返回条数，默认 20"},
			}, nil),
		mk("create_bookmark", "在导航里新增一个网址书签。用户说「收藏这个网址」「把这个网站加进去」时用它。",
			map[string]interface{}{
				"title":       strProp("书签标题（必填）"),
				"url":         strProp("网址，带 http(s)://（必填）"),
				"category_id": strProp("分类 ID 或分类名称，不填则放第一个分类"),
				"description": strProp("备注说明（可选）"),
			}, []string{"title", "url"}),
		mk("update_bookmark", "修改一个已收藏的书签（标题、网址、备注，或移动到别的分类）。",
			map[string]interface{}{
				"id":          strProp("书签 ID（必填，先用 search_bookmarks 或 list_bookmarks 查到）"),
				"title":       strProp("新标题（可选）"),
				"url":         strProp("新网址（可选）"),
				"description": strProp("新备注（可选，传空字符串可清空）"),
				"category_id": strProp("目标分类 ID（可选，移动分类时用）"),
			}, []string{"id"}),
		mk("delete_bookmark", "删除一个已收藏的书签。用户说「删掉这个网址」时用它，删除前最好先跟用户确认。",
			map[string]interface{}{
				"id": strProp("书签 ID（必填，先用 search_bookmarks 查到）"),
			}, []string{"id"}),
	}
}

const aiSystemPrompt = `你是「云栖阁」导航面板的 AI 助手，帮用户找网址、管理收藏、答问题。
规则：
1. 必须用简洁的大白话中文回答，不用英文、不拽术语。
2. 用户问导航里有没有某个网站、找网址时，先调用 search_bookmarks 或 list_categories 查真实数据，再按查到的结果回答；不要编造网址。
3. 用户说收藏/增加网址时，用 create_bookmark（标题和网址必填，分类可按用户说的名称传）。
4. 用户说删除网址时，先用 search_bookmarks 找到对应书签的 ID，跟用户确认后再调用 delete_bookmark。
5. 用户说改网址信息时，用 update_bookmark。
6. 回答要短，能一句话说清就不要说三句。操作成功后用一句话告诉用户结果。`

type aiChatResp struct {
	Choices []struct {
		Message struct {
			Role      string       `json:"role"`
			Content   string       `json:"content"`
			ToolCalls []aiToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func aiPostChatCompletions(ctx context.Context, cfg aiConfig, payload map[string]interface{}) (*aiChatResp, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	transport := &http.Transport{}
	if cfg.ProxyEnabled && strings.TrimSpace(cfg.ProxyURL) != "" {
		if proxyURL, err := url.Parse(strings.TrimSpace(cfg.ProxyURL)); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}
	client := &http.Client{Transport: transport, Timeout: aiTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("上游返回 %d：%s", resp.StatusCode, truncateRunes(strings.TrimSpace(string(respBody)), 300))
	}
	if len(bytes.TrimSpace(respBody)) == 0 {
		return nil, errors.New("上游返回了空响应，请稍后重试")
	}
	var parsed aiChatResp
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, errors.New("上游响应格式看不懂，请稍后重试")
	}
	if len(parsed.Choices) == 0 {
		return nil, errors.New("上游没有返回任何回答，请稍后重试")
	}
	return &parsed, nil
}

// aiRunLocalTool 在本地执行 AI 的 function tool，返回喂给模型的文本结果。
func aiRunLocalTool(username, name, argsJSON string) string {
	var args map[string]interface{}
	_ = json.Unmarshal([]byte(argsJSON), &args)
	if args == nil {
		args = map[string]interface{}{}
	}
	// category_id 允许传分类名称：自动匹配成 ID
	if catName, ok := args["category_id"].(string); ok && strings.TrimSpace(catName) != "" {
		if resolved := aiResolveCategoryID(username, strings.TrimSpace(catName)); resolved != "" {
			args["category_id"] = resolved
		}
	}
	switch name {
	case "search_bookmarks":
		text, _ := mcpSearchBookmarks(username, mcpStrArg(args, "query"), 20)
		return text
	case "list_categories":
		text, _ := mcpListCategories(username)
		return text
	case "list_bookmarks":
		limit := mcpIntArg(args, "limit", 20)
		text, _ := mcpListBookmarks(username, mcpStrArg(args, "category_id"), limit)
		return text
	case "create_bookmark":
		text, _ := mcpCreateBookmark(username, args)
		return text
	case "update_bookmark":
		text, _ := mcpUpdateBookmark(username, args)
		return text
	case "delete_bookmark":
		text, _ := mcpDeleteBookmark(username, mcpStrArg(args, "id"))
		return text
	default:
		return "没有这个工具"
	}
}

// aiResolveCategoryID：分类名称→ID 模糊匹配，传的已经是 ID 则原样返回。
func aiResolveCategoryID(username, nameOrID string) string {
	groups, err := mcpLoadGroups(username)
	if err != nil {
		return ""
	}
	for _, g := range groups {
		if g.ID == nameOrID {
			return g.ID
		}
	}
	for _, g := range groups {
		if strings.Contains(g.Title, nameOrID) || strings.Contains(nameOrID, g.Title) {
			return g.ID
		}
	}
	return ""
}

// AIAsk AI 对话主流程：两轮 function-calling。username 决定查谁的书签。
func AIAsk(username, message string) (string, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return "", errors.New("问题不能为空")
	}
	cfg := loadAIConfig()
	if strings.TrimSpace(cfg.APIKey) == "" {
		return "", errors.New("还没配置 AI 密钥，请先到 AI 设置页填写后再试")
	}
	if username == "" {
		username = "admin"
	}

	ctx, cancel := context.WithTimeout(context.Background(), aiTimeout)
	defer cancel()

	// 第一轮：带 tools，让模型决定要不要查书签
	round1 := map[string]interface{}{
		"model":       cfg.Model,
		"temperature": 0.3,
		"messages": []aiChatMessage{
			{Role: "system", Content: aiSystemPrompt},
			{Role: "user", Content: message},
		},
		"tools":       aiFunctionTools(),
		"tool_choice": "auto",
	}
	resp1, err := aiPostChatCompletions(ctx, cfg, round1)
	if err != nil {
		return "", err
	}
	msg1 := resp1.Choices[0].Message
	if len(msg1.ToolCalls) == 0 {
		// 模型直接回答了
		if strings.TrimSpace(msg1.Content) == "" {
			return "", errors.New("AI 这次没说话，请换个问法再试")
		}
		return strings.TrimSpace(msg1.Content), nil
	}

	// 第二轮：把本地工具结果喂回去，让模型组织最终回答
	messages := []aiChatMessage{
		{Role: "system", Content: aiSystemPrompt},
		{Role: "user", Content: message},
		{Role: "assistant", Content: msg1.Content, ToolCalls: msg1.ToolCalls},
	}
	for _, tc := range msg1.ToolCalls {
		toolResult := aiRunLocalTool(username, tc.Function.Name, tc.Function.Arguments)
		messages = append(messages, aiChatMessage{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    toolResult,
		})
	}
	round2 := map[string]interface{}{
		"model":       cfg.Model,
		"temperature": 0.3,
		"messages":    messages,
	}
	resp2, err := aiPostChatCompletions(ctx, cfg, round2)
	if err != nil {
		return "", err
	}
	reply := strings.TrimSpace(resp2.Choices[0].Message.Content)
	if reply == "" {
		return "", errors.New("AI 这次没说话，请换个问法再试")
	}
	return reply, nil
}

// POST /api/ai/chat（JWT）{"message":"..."} -> {"reply":"..."}
func AIChat(c *gin.Context) {
	username := c.GetString("username")
	var req struct {
		Message string `json:"message"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式不对"})
		return
	}
	reply, err := AIAsk(username, req.Message)
	if err != nil {
		// 密钥没配是 400（前端好跳设置页），其余是 502
		status := http.StatusBadGateway
		if strings.Contains(err.Error(), "还没配置 AI 密钥") {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"reply": reply})
}
