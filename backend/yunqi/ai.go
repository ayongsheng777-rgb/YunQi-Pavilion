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
	aiTimeout        = 60 * time.Second
)

type aiConfig struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

var aiConfigMu sync.Mutex

func aiConfigFile() string { return filepath.Join(YunqiDir(), "ai.json") }

func loadAIConfig() aiConfig {
	aiConfigMu.Lock()
	defer aiConfigMu.Unlock()
	cfg := aiConfig{BaseURL: defaultAIBaseURL, Model: defaultAIModel}
	_ = utils.ReadJSON(aiConfigFile(), &cfg)
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = defaultAIBaseURL
	}
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = defaultAIModel
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
		"base_url": cfg.BaseURL,
		"model":    cfg.Model,
		"has_key":  strings.TrimSpace(cfg.APIKey) != "",
	})
}

type aiConfigReq struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

// POST /api/ai/config（JWT）{"base_url","api_key","model"}；api_key 留空表示保留原值。
func AISetConfig(c *gin.Context) {
	var req aiConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式不对"})
		return
	}
	cfg := loadAIConfig()
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
	search := aiToolDef{Type: "function"}
	search.Function.Name = "search_bookmarks"
	search.Function.Description = "在用户的网址导航里按关键词搜索书签，返回标题和网址。当用户问某个网站、找网址时用它。"
	search.Function.Parameters = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{"type": "string", "description": "搜索关键词"},
		},
		"required": []string{"query"},
	}
	cats := aiToolDef{Type: "function"}
	cats.Function.Name = "list_categories"
	cats.Function.Description = "列出用户导航里的全部分类/分组及书签数量。当用户问有哪些分类、导航里有什么时用它。"
	cats.Function.Parameters = map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
	return []aiToolDef{search, cats}
}

const aiSystemPrompt = `你是「云栖阁」导航面板的 AI 助手，帮用户找网址、答问题。
规则：
1. 必须用简洁的大白话中文回答，不用英文、不拽术语。
2. 用户问导航里有没有某个网站、找网址时，先调用 search_bookmarks 或 list_categories 查真实数据，再按查到的结果回答；不要编造网址。
3. 回答要短，能一句话说清就不要说三句。`

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
	client := &http.Client{}
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
	switch name {
	case "search_bookmarks":
		var args struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal([]byte(argsJSON), &args)
		text, _ := mcpSearchBookmarks(username, strings.TrimSpace(args.Query), 20)
		return text
	case "list_categories":
		text, _ := mcpListCategories(username)
		return text
	default:
		return "没有这个工具"
	}
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
