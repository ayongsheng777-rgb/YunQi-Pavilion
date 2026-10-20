package yunqi

import (
	"flatnasgo-backend/utils"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// 模块 1：网站签到提醒
// 数据：config.DataDir/yunqi/checkin.json（站点清单，按用户名分）
//      config.DataDir/yunqi/checkin_records.json（按日期记录签到）
// 全部接口走 JWT 鉴权，用户名从 token 的 username claim 取。
// ---------------------------------------------------------------------------

// CheckinSite 签到站点
type CheckinSite struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Icon    string `json:"icon"`
	Note    string `json:"note"`
	Enabled bool   `json:"enabled"`
}

type checkinStore struct {
	Sites map[string][]CheckinSite `json:"sites"` // username -> sites
}

// checkinRecords 日期 -> 用户名 -> 已签到站点 id 列表
type checkinRecords struct {
	Dates map[string]map[string][]string `json:"dates"`
}

var checkinMu sync.Mutex

// 路径必须在使用时现算：包级变量在 config.Init() 之前求值，那时 DataDir 还是空的，
// 算出的相对路径会写进容器临时层，重建容器就全丢（已踩过这个坑）。
func checkinFilePath() string     { return filepath.Join(YunqiDir(), "checkin.json") }
func checkinRecordFnPath() string { return filepath.Join(YunqiDir(), "checkin_records.json") }

func todayStr() string { return time.Now().Format("2006-01-02") }

func loadCheckinStore() checkinStore {
	var st checkinStore
	_ = utils.ReadJSON(checkinFilePath(), &st)
	if st.Sites == nil {
		st.Sites = map[string][]CheckinSite{}
	}
	return st
}

func saveCheckinStore(st checkinStore) error {
	return utils.WriteJSON(checkinFilePath(), st)
}

func loadCheckinRecords() checkinRecords {
	var rc checkinRecords
	_ = utils.ReadJSON(checkinRecordFnPath(), &rc)
	if rc.Dates == nil {
		rc.Dates = map[string]map[string][]string{}
	}
	return rc
}

func saveCheckinRecords(rc checkinRecords) error {
	return utils.WriteJSON(checkinRecordFnPath(), rc)
}

func checkinHasDone(list []string, siteID string) bool {
	for _, id := range list {
		if id == siteID {
			return true
		}
	}
	return false
}

// normalizeCheckinURL 归一化网址：去协议、去 www.、去 query/fragment、去末尾斜杠、转小写后比较。
func normalizeCheckinURL(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.ToLower(s)
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "www.")
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSuffix(s, "/")
	return s
}

// markCheckinDone 标记某用户今日已签到某站点（调用方需持有 checkinMu）。
func markCheckinDoneLocked(rc checkinRecords, username, siteID string) {
	date := todayStr()
	byUser, ok := rc.Dates[date]
	if !ok {
		byUser = map[string][]string{}
		rc.Dates[date] = byUser
	}
	if !checkinHasDone(byUser[username], siteID) {
		byUser[username] = append(byUser[username], siteID)
	}
}

// GET /api/checkin/sites
func CheckinListSites(c *gin.Context) {
	username := c.GetString("username")
	checkinMu.Lock()
	st := loadCheckinStore()
	checkinMu.Unlock()
	sites := st.Sites[username]
	if sites == nil {
		sites = []CheckinSite{}
	}
	c.JSON(http.StatusOK, gin.H{"sites": sites})
}

type checkinSiteOp struct {
	Op   string      `json:"op"`
	Site CheckinSite `json:"site"`
	ID   string      `json:"id"`
}

// POST /api/checkin/sites  body: {"op":"add","site":{...}} / {"op":"update","site":{...带id}} / {"op":"delete","id":"..."}
func CheckinManageSites(c *gin.Context) {
	username := c.GetString("username")
	var req checkinSiteOp
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式不对"})
		return
	}

	checkinMu.Lock()
	defer checkinMu.Unlock()
	st := loadCheckinStore()
	sites := st.Sites[username]

	switch req.Op {
	case "add":
		site := req.Site
		if strings.TrimSpace(site.Name) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "站点名称不能为空"})
			return
		}
		if strings.TrimSpace(site.URL) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "站点网址不能为空"})
			return
		}
		site.ID = newUUID()
		sites = append(sites, site)
		st.Sites[username] = sites
		if err := saveCheckinStore(st); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败，请稍后再试"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "site": site})
	case "update":
		if strings.TrimSpace(req.Site.ID) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "缺少站点 id"})
			return
		}
		found := false
		for i := range sites {
			if sites[i].ID == req.Site.ID {
				updated := req.Site
				updated.ID = sites[i].ID
				sites[i] = updated
				found = true
				break
			}
		}
		if !found {
			c.JSON(http.StatusNotFound, gin.H{"error": "找不到这个站点"})
			return
		}
		st.Sites[username] = sites
		if err := saveCheckinStore(st); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败，请稍后再试"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "site": req.Site})
	case "delete":
		if strings.TrimSpace(req.ID) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "缺少站点 id"})
			return
		}
		kept := sites[:0:0]
		found := false
		for _, s := range sites {
			if s.ID == req.ID {
				found = true
				continue
			}
			kept = append(kept, s)
		}
		if !found {
			c.JSON(http.StatusNotFound, gin.H{"error": "找不到这个站点"})
			return
		}
		if kept == nil {
			kept = []CheckinSite{}
		}
		st.Sites[username] = kept
		if err := saveCheckinStore(st); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败，请稍后再试"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的操作"})
	}
}

// POST /api/checkin/open  body: {"url":"..."} 或 {"site_id":"..."}
// 记录今日已签到；url 用归一化后比较。
func CheckinOpen(c *gin.Context) {
	username := c.GetString("username")
	var req struct {
		URL    string `json:"url"`
		SiteID string `json:"site_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式不对"})
		return
	}

	checkinMu.Lock()
	defer checkinMu.Unlock()
	st := loadCheckinStore()
	sites := st.Sites[username]

	var matched *CheckinSite
	if strings.TrimSpace(req.SiteID) != "" {
		for i := range sites {
			if sites[i].ID == req.SiteID {
				matched = &sites[i]
				break
			}
		}
	} else if strings.TrimSpace(req.URL) != "" {
		norm := normalizeCheckinURL(req.URL)
		for i := range sites {
			if normalizeCheckinURL(sites[i].URL) == norm {
				matched = &sites[i]
				break
			}
		}
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请传入 url 或 site_id"})
		return
	}

	if matched == nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "matched": false})
		return
	}
	rc := loadCheckinRecords()
	markCheckinDoneLocked(rc, username, matched.ID)
	if err := saveCheckinRecords(rc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "记录签到失败，请稍后再试"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "matched": true, "site_id": matched.ID})
}

// GET /api/checkin/go/:id  先标记今日已签到，再 302 跳到站点网址
func CheckinGo(c *gin.Context) {
	username := c.GetString("username")
	id := c.Param("id")

	checkinMu.Lock()
	defer checkinMu.Unlock()
	st := loadCheckinStore()
	var target *CheckinSite
	for i := range st.Sites[username] {
		if st.Sites[username][i].ID == id {
			target = &st.Sites[username][i]
			break
		}
	}
	if target == nil || strings.TrimSpace(target.URL) == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "找不到这个站点"})
		return
	}
	rc := loadCheckinRecords()
	markCheckinDoneLocked(rc, username, target.ID)
	_ = saveCheckinRecords(rc)
	c.Redirect(http.StatusFound, target.URL)
}

// GET /api/checkin/status  今日各站点签到状态（按自然日自动重置）
func CheckinStatus(c *gin.Context) {
	username := c.GetString("username")
	checkinMu.Lock()
	st := loadCheckinStore()
	rc := loadCheckinRecords()
	checkinMu.Unlock()

	doneIDs := rc.Dates[todayStr()][username]
	status := []gin.H{}
	for _, s := range st.Sites[username] {
		status = append(status, gin.H{
			"site_id": s.ID,
			"name":    s.Name,
			"url":     s.URL,
			"done":    checkinHasDone(doneIDs, s.ID),
		})
	}
	c.JSON(http.StatusOK, gin.H{"date": todayStr(), "status": status})
}

// GET /api/checkin/search?q=xxx
// 在已收藏的书签里按标题/网址搜索，供签到站点表单自动匹配填充。
func CheckinSearchBookmarks(c *gin.Context) {
	username := c.GetString("username")
	q := strings.TrimSpace(strings.ToLower(c.Query("q")))
	if q == "" {
		c.JSON(http.StatusOK, gin.H{"results": []gin.H{}})
		return
	}
	groups, err := loadBookmarkGroups(UserDataFile(username))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"results": []gin.H{}})
		return
	}
	results := []gin.H{}
	for _, g := range groups {
		for _, it := range g.Items {
			if strings.Contains(strings.ToLower(it.Title), q) || strings.Contains(strings.ToLower(it.Url), q) {
				results = append(results, gin.H{
					"id":       it.ID,
					"title":    it.Title,
					"url":      it.Url,
					"icon":     it.Icon,
					"category": g.Title,
				})
				if len(results) >= 10 {
					break
				}
			}
		}
		if len(results) >= 10 {
			break
		}
	}
	c.JSON(http.StatusOK, gin.H{"results": results})
}
