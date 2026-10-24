package yunqi

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"flatnasgo-backend/utils"

	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// Horizon 日报：OECT NAS 上的 Horizon 每天 08:00 跑完后，把中文日报 Markdown
// POST 到这里，存为 data/yunqi/horizon/<日期>.md，并更新 index.json；
// 前端卡片经 GET /api/horizon/list 取日期列表、GET /api/horizon/day 取全文。
// 推送走 OpenAPI token（与 IT 日报同一鉴权），查询走 JWT（登录后可见）。
// ---------------------------------------------------------------------------

func horizonDir() string {
	return filepath.Join(YunqiDir(), "horizon")
}

func horizonFile(date string) string {
	return filepath.Join(horizonDir(), date+".md")
}

func horizonIndexFile() string {
	return filepath.Join(horizonDir(), "index.json")
}

var horizonDateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
var horizonHeadlineRe = regexp.MustCompile(`(?m)^### \[`)

// countHeadlines 粗数正文里 ### 标题行数，即精选条数。
func countHeadlines(md string) int {
	return len(horizonHeadlineRe.FindAllString(md, -1))
}

type horizonIndex struct {
	Dates  []string       `json:"dates"`
	Latest string         `json:"latest"`
	Counts map[string]int `json:"counts"`
}

// readHorizonIndex 读 index.json；没有则扫描目录重建（容错：直接拷文件也能认）。
func readHorizonIndex() horizonIndex {
	var idx horizonIndex
	if err := utils.ReadJSON(horizonIndexFile(), &idx); err == nil && len(idx.Dates) > 0 {
		if idx.Counts == nil {
			idx.Counts = map[string]int{}
		}
		return idx
	}
	idx = horizonIndex{Dates: []string{}, Counts: map[string]int{}}
	entries, err := os.ReadDir(horizonDir())
	if err != nil {
		return idx
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") {
			continue
		}
		date := strings.TrimSuffix(name, ".md")
		if !horizonDateRe.MatchString(date) {
			continue
		}
		idx.Dates = append(idx.Dates, date)
		if b, err := os.ReadFile(horizonFile(date)); err == nil {
			idx.Counts[date] = countHeadlines(string(b))
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(idx.Dates)))
	if len(idx.Dates) > 0 {
		idx.Latest = idx.Dates[0]
	}
	return idx
}

func writeHorizonIndex(idx horizonIndex) {
	_ = os.MkdirAll(horizonDir(), 0755)
	_ = utils.WriteJSON(horizonIndexFile(), idx)
}

// POST /openapi/v1/horizon/push（OpenAPI token 鉴权，供 OECT 调用）
// Body: {"date": "2026-10-24", "markdown": "# Horizon 每日速递 ..."}
func HorizonPush(c *gin.Context) {
	var payload struct {
		Date     string `json:"date"`
		Markdown string `json:"markdown"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式不对"})
		return
	}
	if !horizonDateRe.MatchString(payload.Date) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "日期格式不对，应为 YYYY-MM-DD"})
		return
	}
	if strings.TrimSpace(payload.Markdown) == "" || len(payload.Markdown) > 512*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "日报内容为空或过大"})
		return
	}
	_ = os.MkdirAll(horizonDir(), 0755)
	if err := os.WriteFile(horizonFile(payload.Date), []byte(payload.Markdown), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存日报失败"})
		return
	}
	idx := readHorizonIndex()
	found := false
	for _, d := range idx.Dates {
		if d == payload.Date {
			found = true
			break
		}
	}
	if !found {
		idx.Dates = append(idx.Dates, payload.Date)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(idx.Dates)))
	idx.Latest = idx.Dates[0]
	if idx.Counts == nil {
		idx.Counts = map[string]int{}
	}
	idx.Counts[payload.Date] = countHeadlines(payload.Markdown)
	writeHorizonIndex(idx)
	c.JSON(http.StatusOK, gin.H{"success": true, "date": payload.Date, "count": idx.Counts[payload.Date]})
}

// GET /api/horizon/list（JWT 鉴权，供前端卡片读取）
func HorizonList(c *gin.Context) {
	idx := readHorizonIndex()
	c.JSON(http.StatusOK, gin.H{
		"exists": len(idx.Dates) > 0,
		"dates":  idx.Dates,
		"latest": idx.Latest,
		"counts": idx.Counts,
	})
}

// GET /api/horizon/day?date=YYYY-MM-DD（JWT 鉴权，供全文阅读页读取；不传则取最新）
func HorizonDay(c *gin.Context) {
	idx := readHorizonIndex()
	date := strings.TrimSpace(c.Query("date"))
	if date == "" {
		date = idx.Latest
	}
	if date == "" || !horizonDateRe.MatchString(date) {
		c.JSON(http.StatusOK, gin.H{"exists": false})
		return
	}
	b, err := os.ReadFile(horizonFile(date))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"exists": false, "date": date})
		return
	}
	c.JSON(http.StatusOK, gin.H{"exists": true, "date": date, "markdown": string(b)})
}
