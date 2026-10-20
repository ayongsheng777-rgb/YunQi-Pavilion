package yunqi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"flatnasgo-backend/utils"

	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// 音乐模块（LX Music 集成）：
//   - 云栖阁后端把前端的音乐请求代理给同项目的 music-engine 服务
//     （搜歌/歌词/榜单/歌单/音源脚本沙盒解析播放地址）
//   - 下载：后端把解析出的播放地址直接下载到 NAS 指定目录
//   - 配置存 data/yunqi/music_config.json
// ---------------------------------------------------------------------------

type MusicConfig struct {
	EngineURL      string `json:"engineUrl"`      // 音乐引擎地址（同 compose 网络内用 http://music-engine:23400）
	ProxyEnabled   bool   `json:"proxyEnabled"`   // 是否给引擎开代理
	ProxyHost      string `json:"proxyHost"`      // 代理主机（默认 NAS 本机 sing-box）
	ProxyPort      int    `json:"proxyPort"`      // 代理端口
	DownloadDir    string `json:"downloadDir"`    // 容器内下载目录（compose 挂载 NAS 目录到 /downloads）
	DefaultSource  string `json:"defaultSource"`  // 默认音源平台 kw/kg/tx/wy/mg
	DefaultQuality string `json:"defaultQuality"` // 默认音质 128k/320k/flac/flac24bit
	DefaultMode    string `json:"defaultMode"`    // 默认模式 play/download/playDownload
}

func musicConfigFile() string {
	return filepath.Join(YunqiDir(), "music_config.json")
}

func defaultMusicConfig() MusicConfig {
	return MusicConfig{
		EngineURL:      "http://music-engine:23400",
		ProxyEnabled:   true,
		ProxyHost:      "192.168.57.228",
		ProxyPort:      7890,
		DownloadDir:    "/downloads",
		DefaultSource:  "kw",
		DefaultQuality: "320k",
		DefaultMode:    "play",
	}
}

func loadMusicConfig() MusicConfig {
	cfg := defaultMusicConfig()
	var saved MusicConfig
	if err := utils.ReadJSON(musicConfigFile(), &saved); err == nil {
		if saved.EngineURL != "" {
			cfg.EngineURL = saved.EngineURL
		}
		cfg.ProxyEnabled = saved.ProxyEnabled
		if saved.ProxyHost != "" {
			cfg.ProxyHost = saved.ProxyHost
		}
		if saved.ProxyPort > 0 {
			cfg.ProxyPort = saved.ProxyPort
		}
		if saved.DownloadDir != "" {
			cfg.DownloadDir = saved.DownloadDir
		}
		if saved.DefaultSource != "" {
			cfg.DefaultSource = saved.DefaultSource
		}
		if saved.DefaultQuality != "" {
			cfg.DefaultQuality = saved.DefaultQuality
		}
		if saved.DefaultMode != "" {
			cfg.DefaultMode = saved.DefaultMode
		}
	}
	return cfg
}

func saveMusicConfig(cfg MusicConfig) error {
	_ = os.MkdirAll(filepath.Dir(musicConfigFile()), 0755)
	return utils.WriteJSON(musicConfigFile(), cfg)
}

// 把代理配置推给引擎（引擎运行时生效）
func pushProxyToEngine(cfg MusicConfig) {
	body := map[string]interface{}{"proxy": nil}
	if cfg.ProxyEnabled {
		body["proxy"] = map[string]interface{}{"host": cfg.ProxyHost, "port": cfg.ProxyPort}
	}
	data, _ := json.Marshal(body)
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("POST", strings.TrimRight(cfg.EngineURL, "/")+"/api/config", bytes.NewReader(data))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

// ---- 配置接口 ----

// GET /api/music/config
func MusicGetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"config": loadMusicConfig()})
}

// POST /api/music/config
func MusicSaveConfig(c *gin.Context) {
	var cfg MusicConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "配置格式不对"})
		return
	}
	def := defaultMusicConfig()
	if cfg.EngineURL == "" {
		cfg.EngineURL = def.EngineURL
	}
	if cfg.ProxyHost == "" {
		cfg.ProxyHost = def.ProxyHost
	}
	if cfg.ProxyPort <= 0 {
		cfg.ProxyPort = def.ProxyPort
	}
	if cfg.DownloadDir == "" {
		cfg.DownloadDir = def.DownloadDir
	}
	if cfg.DefaultSource == "" {
		cfg.DefaultSource = def.DefaultSource
	}
	if cfg.DefaultQuality == "" {
		cfg.DefaultQuality = def.DefaultQuality
	}
	if cfg.DefaultMode == "" {
		cfg.DefaultMode = def.DefaultMode
	}
	if err := saveMusicConfig(cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存配置失败"})
		return
	}
	pushProxyToEngine(cfg)
	c.JSON(http.StatusOK, gin.H{"success": true, "config": cfg})
}

// ---- 引擎代理 ----

var engineClient = &http.Client{Timeout: 60 * time.Second}

// engineRequest 构造向引擎的请求，path 形如 /api/search
func engineRequest(cfg MusicConfig, method, path string, query url.Values, body io.Reader, contentType string) (*http.Response, error) {
	u := strings.TrimRight(cfg.EngineURL, "/") + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequest(method, u, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return engineClient.Do(req)
}

// musicProxy 把某个引擎接口原样代理给前端
func MusicProxyEngine(enginePath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := loadMusicConfig()
		var body io.Reader
		var contentType string
		if c.Request.Method == "POST" {
			data, _ := io.ReadAll(c.Request.Body)
			body = bytes.NewReader(data)
			contentType = "application/json"
		}
		path := enginePath
		// 路径参数 :id 替换
		if id := c.Param("id"); id != "" {
			path = strings.Replace(path, ":id", url.PathEscape(id), 1)
		}
		resp, err := engineRequest(cfg, c.Request.Method, path, c.Request.URL.Query(), body, contentType)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 502, "msg": "音乐引擎连不上：" + err.Error()})
			return
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		c.Data(http.StatusOK, "application/json; charset=utf-8", data)
	}
}

// ---- 下载 ----

type DownloadTask struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Singer    string    `json:"singer"`
	Source    string    `json:"source"`
	Quality   string    `json:"quality"`
	Status    string    `json:"status"` // downloading / done / failed
	Progress  int64     `json:"progress"`
	Total     int64     `json:"total"`
	File      string    `json:"file"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"startedAt"`
}

var (
	downloadMu    sync.Mutex
	downloadTasks = map[string]*DownloadTask{}
	downloadOrder []string
)

func qualityExt(quality string) string {
	if strings.HasPrefix(quality, "flac") {
		return ".flac"
	}
	return ".mp3"
}

// cleanFileName 去掉文件名非法字符
func cleanFileName(name string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	name = strings.TrimSpace(replacer.Replace(name))
	if len([]rune(name)) > 120 {
		name = string([]rune(name)[:120])
	}
	if name == "" {
		name = "music"
	}
	return name
}

// POST /api/music/download  { source, musicInfo, quality }
func MusicDownload(c *gin.Context) {
	var req struct {
		Source    string                 `json:"source"`
		MusicInfo map[string]interface{} `json:"musicInfo"`
		Quality   string                 `json:"quality"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.MusicInfo == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数不对"})
		return
	}
	cfg := loadMusicConfig()
	if req.Quality == "" {
		req.Quality = cfg.DefaultQuality
	}
	name, _ := req.MusicInfo["name"].(string)
	singer, _ := req.MusicInfo["singer"].(string)
	task := &DownloadTask{
		ID:        fmt.Sprintf("dl-%d", time.Now().UnixNano()),
		Name:      name,
		Singer:    singer,
		Source:    req.Source,
		Quality:   req.Quality,
		Status:    "downloading",
		StartedAt: time.Now(),
	}
	downloadMu.Lock()
	downloadTasks[task.ID] = task
	downloadOrder = append([]string{task.ID}, downloadOrder...)
	if len(downloadOrder) > 50 {
		downloadOrder = downloadOrder[:50]
	}
	downloadMu.Unlock()

	go runDownload(cfg, task, req.Source, req.MusicInfo)

	c.JSON(http.StatusOK, gin.H{"success": true, "taskId": task.ID})
}

func runDownload(cfg MusicConfig, task *DownloadTask, source string, musicInfo map[string]interface{}) {
	fail := func(msg string) {
		downloadMu.Lock()
		task.Status = "failed"
		task.Error = msg
		downloadMu.Unlock()
	}
	// 1. 问引擎要播放地址
	payload, _ := json.Marshal(map[string]interface{}{"source": source, "musicInfo": musicInfo, "quality": task.Quality})
	resp, err := engineRequest(cfg, "POST", "/api/music-url", nil, bytes.NewReader(payload), "application/json")
	if err != nil {
		fail("音乐引擎连不上")
		return
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var engineResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if json.Unmarshal(data, &engineResp) != nil || engineResp.Code != 0 || engineResp.Data.URL == "" {
		msg := engineResp.Msg
		if msg == "" {
			msg = "拿不到播放地址"
		}
		fail(msg)
		return
	}
	// 2. 下载到 NAS 目录。设置里的写法：
	//   空 = /downloads；vol1/... 或 vol2/... 开头 = 对应磁盘下的任意路径；
	//   以 / 开头 = 容器内绝对路径原样用；只填名字 = /downloads 下的子目录。
	downloadDir := strings.TrimSpace(cfg.DownloadDir)
	switch {
	case downloadDir == "":
		downloadDir = "/downloads"
	case downloadDir == "vol1" || downloadDir == "vol2" ||
		strings.HasPrefix(downloadDir, "vol1/") || strings.HasPrefix(downloadDir, "vol2/"):
		downloadDir = filepath.Join("/nas", downloadDir)
	case filepath.IsAbs(downloadDir):
		// 原样用
	default:
		downloadDir = filepath.Join("/downloads", downloadDir)
	}
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		fail("下载目录打不开：" + downloadDir)
		return
	}
	fileName := cleanFileName(task.Name+" - "+task.Singer) + qualityExt(task.Quality)
	filePath := filepath.Join(downloadDir, fileName)
	downloadMu.Lock()
	task.File = fileName
	downloadMu.Unlock()

	// 下载走配置的代理（音源地址多半要走代理才通；和引擎验真用的是同一条路）
	dlClient := &http.Client{}
	if cfg.ProxyEnabled && cfg.ProxyHost != "" {
		proxyURL, perr := url.Parse(fmt.Sprintf("http://%s:%d", cfg.ProxyHost, cfg.ProxyPort))
		if perr == nil {
			dlClient.Transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
		}
	}
	dlResp, err := dlClient.Get(engineResp.Data.URL)
	if err != nil {
		fail("下载失败")
		return
	}
	defer dlResp.Body.Close()
	if dlResp.StatusCode != 200 {
		fail(fmt.Sprintf("下载失败（%d）", dlResp.StatusCode))
		return
	}
	task.Total = dlResp.ContentLength
	f, err := os.Create(filePath)
	if err != nil {
		fail("写文件失败")
		return
	}
	defer f.Close()
	buf := make([]byte, 64*1024)
	for {
		n, rerr := dlResp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				fail("写文件失败")
				return
			}
			downloadMu.Lock()
			task.Progress += int64(n)
			downloadMu.Unlock()
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			fail("下载中断")
			return
		}
	}
	downloadMu.Lock()
	task.Status = "done"
	task.Progress = task.Total
	downloadMu.Unlock()
}

// GET /api/music/download/tasks
func MusicDownloadTasks(c *gin.Context) {
	downloadMu.Lock()
	defer downloadMu.Unlock()
	list := []*DownloadTask{}
	for _, id := range downloadOrder {
		if t, ok := downloadTasks[id]; ok {
			list = append(list, t)
		}
	}
	c.JSON(http.StatusOK, gin.H{"tasks": list})
}
