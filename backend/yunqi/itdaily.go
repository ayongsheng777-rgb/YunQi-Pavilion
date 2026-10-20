package yunqi

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

	"flatnasgo-backend/utils"

	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// IT&AI 日报：n8n 工作流每天 19 点生成后 POST 到这里，存为 data/yunqi/it_daily.json，
// 前端小组件经 GET /api/itdaily/latest 读取展示。
// ---------------------------------------------------------------------------

func itDailyFile() string {
	return filepath.Join(YunqiDir(), "it_daily.json")
}

// POST /openapi/v1/itdaily/push（OpenAPI token 鉴权，供 n8n 调用）
// Body 为日报 JSON（reportDate/count/sections/bugs/trend 等字段）。
func ITDailyPush(c *gin.Context) {
	var payload map[string]interface{}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式不对"})
		return
	}
	if payload == nil {
		payload = map[string]interface{}{}
	}
	payload["receivedAt"] = time.Now().Format(time.RFC3339)

	dir := filepath.Dir(itDailyFile())
	_ = os.MkdirAll(dir, 0755)
	if err := utils.WriteJSON(itDailyFile(), payload); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存日报失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GET /api/itdaily/latest（JWT 鉴权，供前端小组件读取）
func ITDailyLatest(c *gin.Context) {
	var data map[string]interface{}
	if err := utils.ReadJSON(itDailyFile(), &data); err != nil || data == nil {
		c.JSON(http.StatusOK, gin.H{"exists": false})
		return
	}
	data["exists"] = true
	c.JSON(http.StatusOK, data)
}
