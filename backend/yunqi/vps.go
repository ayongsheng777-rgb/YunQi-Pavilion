package yunqi

// VPS 监控：通过各 VPS 的 1Panel 面板 API 采集数据（不用 SSH）。
// - 节点配置存 data/yunqi/vps_nodes.json（名称、面板地址、API 密钥、是否走代理）
// - 后台协程每 20 秒轮询一次：系统信息(current) + 容器列表(containers)，OS 信息只取一次
// - 采集走 J4125 sing-box 代理（海外面板端口国内直连常被掐），代理失败自动回退直连
// - 前端只读 /api/vps/status（不含密钥）；节点管理走 /api/vps/nodes（密钥只写不读）

import (
	"bytes"
	"crypto/md5"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const vpsProxyURL = "http://192.168.57.228:7890" // J4125 sing-box，与音乐/AI 共用
const vpsPollInterval = 20 * time.Second

type VpsNode struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	BaseURL  string `json:"baseUrl"`
	APIKey   string `json:"apiKey"`
	UseProxy bool   `json:"useProxy"`
}

type vpsConfigFile struct {
	Nodes []VpsNode `json:"nodes"`
}

type VpsContainer struct {
	Name  string `json:"name"`
	State string `json:"state"`
	Image string `json:"image"`
}

type VpsNodeStatus struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	Host              string         `json:"host"`
	Online            bool           `json:"online"`
	Error             string         `json:"error,omitempty"`
	LatencyMs         int            `json:"latencyMs"`
	UpdatedAt         int64          `json:"updatedAt"` // unix ms
	OS                string         `json:"os"`
	Cores             int            `json:"cores"`
	CPUPercent        float64        `json:"cpuPercent"`
	MemPercent        float64        `json:"memPercent"`
	MemUsed           int64          `json:"memUsed"`
	MemTotal          int64          `json:"memTotal"`
	DiskPercent       float64        `json:"diskPercent"`
	DiskUsed          int64          `json:"diskUsed"`
	DiskTotal         int64          `json:"diskTotal"`
	Load1             float64        `json:"load1"`
	UptimeSec         int64          `json:"uptimeSec"`
	NetUpRate         float64        `json:"netUpRate"`   // bytes/s
	NetDownRate       float64        `json:"netDownRate"` // bytes/s
	PanelURL          string         `json:"panelUrl"`
	SSHLogAvailable   bool           `json:"sshLogAvailable"`
	SSHFailed24h      int            `json:"sshFailed24h"`
	SSHLastIP         string         `json:"sshLastIp"`
	Containers        []VpsContainer `json:"containers"`
	ContainersRunning int            `json:"containersRunning"`
	ContainersTotal   int            `json:"containersTotal"`
	ProxyContainer    string         `json:"proxyContainer"` // sing-box 容器状态，无则空
}

var (
	vpsMu       sync.RWMutex
	vpsStatuses = map[string]*VpsNodeStatus{}
	vpsNetPrev  = map[string]vpsNetSample{}
	vpsStarted  bool
)

type vpsNetSample struct {
	sent, recv int64
	at         time.Time
}

func vpsNodesPath() string { return filepath.Join(YunqiDir(), "vps_nodes.json") }

func loadVpsNodes() []VpsNode {
	raw, err := os.ReadFile(vpsNodesPath())
	if err != nil {
		return nil
	}
	var cfg vpsConfigFile
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil
	}
	return cfg.Nodes
}

func saveVpsNodes(nodes []VpsNode) error {
	cfg := vpsConfigFile{Nodes: nodes}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(vpsNodesPath(), raw, 0o600)
}

func normalizePanelBase(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return strings.TrimRight(s, "/")
	}
	return u.Scheme + "://" + u.Host
}

// ---- 1Panel API 客户端 ----

func panelClient(useProxy bool) *http.Client {
	tr := &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true}, // 面板可能用自签/IP 访问
		DisableKeepAlives: true,
	}
	if useProxy {
		if pu, err := url.Parse(vpsProxyURL); err == nil {
			tr.Proxy = http.ProxyURL(pu)
		}
	}
	return &http.Client{Timeout: 12 * time.Second, Transport: tr}
}

func panelCall(client *http.Client, node VpsNode, method, path string, body any) (int, []byte, int, error) {
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, node.BaseURL+path, reader)
	if err != nil {
		return 0, nil, 0, err
	}
	ts := fmt.Sprintf("%d", time.Now().Unix())
	sum := md5.Sum([]byte("1panel" + node.APIKey + ts))
	req.Header.Set("1Panel-Timestamp", ts)
	req.Header.Set("1Panel-Token", hex.EncodeToString(sum[:]))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	start := time.Now()
	resp, err := client.Do(req)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		return 0, nil, latency, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return resp.StatusCode, raw, latency, nil
}

// panelCallAuto 先按节点设置（代理/直连）调，网络错误时换另一种再试一次。
func panelCallAuto(node VpsNode, method, path string, body any) (int, []byte, int, error) {
	code, raw, lat, err := panelCall(panelClient(node.UseProxy), node, method, path, body)
	if err != nil {
		code2, raw2, lat2, err2 := panelCall(panelClient(!node.UseProxy), node, method, path, body)
		if err2 == nil {
			return code2, raw2, lat2, nil
		}
	}
	return code, raw, lat, err
}

type panelResp struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func fetchNodeStatus(node VpsNode, prev *VpsNodeStatus) *VpsNodeStatus {
	st := &VpsNodeStatus{ID: node.ID, Name: node.Name, PanelURL: node.BaseURL, UpdatedAt: time.Now().UnixMilli()}
	if u, err := url.Parse(node.BaseURL); err == nil {
		st.Host = u.Host
	}
	fail := func(format string, args ...any) *VpsNodeStatus {
		st.Online = false
		st.Error = fmt.Sprintf(format, args...)
		if prev != nil { // 保留上次的 OS/容器等静态信息，界面不至于全空
			st.OS = prev.OS
			st.Cores = prev.Cores
		}
		return st
	}

	// 当前资源
	code, raw, lat, err := panelCallAuto(node, "GET", "/api/v2/dashboard/current/all/all", nil)
	if err != nil {
		return fail("连接失败：%v", err)
	}
	st.LatencyMs = lat
	var pr panelResp
	if err := json.Unmarshal(raw, &pr); err != nil || pr.Code != 200 {
		return fail("接口返回异常（HTTP %d）：%s", code, pr.Message)
	}
	var cur struct {
		CPUUsedPercent    float64 `json:"cpuUsedPercent"`
		CPUTotal          int     `json:"cpuTotal"`
		MemoryUsedPercent float64 `json:"memoryUsedPercent"`
		MemoryUsed        int64   `json:"memoryUsed"`
		MemoryTotal       int64   `json:"memoryTotal"`
		Load1             float64 `json:"load1"`
		Uptime            int64   `json:"uptime"`
		NetBytesSent      int64   `json:"netBytesSent"`
		NetBytesRecv      int64   `json:"netBytesRecv"`
		DiskData          []struct {
			Path        string  `json:"path"`
			Total       int64   `json:"total"`
			Used        int64   `json:"used"`
			UsedPercent float64 `json:"usedPercent"`
		} `json:"diskData"`
	}
	if err := json.Unmarshal(pr.Data, &cur); err != nil {
		return fail("数据解析失败：%v", err)
	}
	st.Online = true
	st.CPUPercent = cur.CPUUsedPercent
	st.Cores = cur.CPUTotal
	st.MemPercent = cur.MemoryUsedPercent
	st.MemUsed, st.MemTotal = cur.MemoryUsed, cur.MemoryTotal
	st.Load1 = cur.Load1
	st.UptimeSec = cur.Uptime
	for _, d := range cur.DiskData {
		if d.Path == "/" {
			st.DiskPercent, st.DiskUsed, st.DiskTotal = d.UsedPercent, d.Used, d.Total
			break
		}
	}
	if st.DiskTotal == 0 && len(cur.DiskData) > 0 {
		d := cur.DiskData[0]
		st.DiskPercent, st.DiskUsed, st.DiskTotal = d.UsedPercent, d.Used, d.Total
	}

	// 网络速率：与上次采样比较
	now := time.Now()
	vpsMu.Lock()
	if prevSample, ok := vpsNetPrev[node.ID]; ok {
		dt := now.Sub(prevSample.at).Seconds()
		if dt > 1 {
			if cur.NetBytesSent >= prevSample.sent {
				st.NetUpRate = float64(cur.NetBytesSent-prevSample.sent) / dt
			}
			if cur.NetBytesRecv >= prevSample.recv {
				st.NetDownRate = float64(cur.NetBytesRecv-prevSample.recv) / dt
			}
		}
	}
	vpsNetPrev[node.ID] = vpsNetSample{sent: cur.NetBytesSent, recv: cur.NetBytesRecv, at: now}
	vpsMu.Unlock()
	if prev != nil && st.NetUpRate == 0 && st.NetDownRate == 0 {
		st.NetUpRate, st.NetDownRate = prev.NetUpRate, prev.NetDownRate
	}

	// OS 信息（只取一次）
	if prev != nil && prev.OS != "" {
		st.OS = prev.OS
	} else if code2, raw2, _, err2 := panelCallAuto(node, "GET", "/api/v2/dashboard/base/os", nil); err2 == nil && code2 == 200 {
		var pr2 panelResp
		if json.Unmarshal(raw2, &pr2) == nil && pr2.Code == 200 {
			var osInfo struct {
				PrettyDistro string `json:"prettyDistro"`
			}
			if json.Unmarshal(pr2.Data, &osInfo) == nil {
				st.OS = osInfo.PrettyDistro
			}
		}
	}

	// SSH 爆破日志（经 1Panel 文件接口读 /var/log/auth.log，约 5 分钟一次）
	if sshDue(node.ID) {
		if cnt, lastIP, ok := fetchSSHLog(node); ok {
			st.SSHLogAvailable = true
			st.SSHFailed24h = cnt
			st.SSHLastIP = lastIP
			vpsMu.Lock()
			vpsSSHCache[node.ID] = sshEntry{at: time.Now(), failed: cnt, lastIP: lastIP, ok: true}
			vpsMu.Unlock()
		} else {
			vpsMu.Lock()
			vpsSSHCache[node.ID] = sshEntry{at: time.Now(), ok: false}
			vpsMu.Unlock()
			if prev != nil { // 一次失败不清掉已有数据
				st.SSHLogAvailable, st.SSHFailed24h, st.SSHLastIP = prev.SSHLogAvailable, prev.SSHFailed24h, prev.SSHLastIP
			}
		}
	} else if prev != nil {
		st.SSHLogAvailable, st.SSHFailed24h, st.SSHLastIP = prev.SSHLogAvailable, prev.SSHFailed24h, prev.SSHLastIP
	}

	// 容器列表
	if _, raw3, _, err3 := panelCallAuto(node, "POST", "/api/v2/containers/search", map[string]any{
		"page": 1, "pageSize": 100, "name": "", "state": "all", "orderBy": "name", "order": "descending",
	}); err3 == nil {
		var pr3 panelResp
		if json.Unmarshal(raw3, &pr3) == nil && pr3.Code == 200 {
			var page struct {
				Items []struct {
					Name      string `json:"name"`
					State     string `json:"state"`
					ImageName string `json:"imageName"`
				} `json:"items"`
			}
			if json.Unmarshal(pr3.Data, &page) == nil {
				st.ContainersTotal = len(page.Items)
				for _, it := range page.Items {
					st.Containers = append(st.Containers, VpsContainer{Name: it.Name, State: it.State, Image: it.ImageName})
					if it.State == "running" {
						st.ContainersRunning++
					}
					if strings.Contains(strings.ToLower(it.Name), "sing-box") {
						st.ProxyContainer = it.State
					}
				}
			}
		}
	}
	return st
}

// ---- 后台采集 ----

func StartVpsCollector() {
	vpsMu.Lock()
	if vpsStarted {
		vpsMu.Unlock()
		return
	}
	vpsStarted = true
	vpsMu.Unlock()
	go func() {
		for {
			nodes := loadVpsNodes()
			for _, node := range nodes {
				vpsMu.RLock()
				prev := vpsStatuses[node.ID]
				vpsMu.RUnlock()
				st := fetchNodeStatus(node, prev)
				vpsMu.Lock()
				vpsStatuses[node.ID] = st
				vpsMu.Unlock()
			}
			time.Sleep(vpsPollInterval)
		}
	}()
}

// ---- HTTP 接口 ----

func VpsGetStatus(c *gin.Context) {
	nodes := loadVpsNodes()
	out := make([]*VpsNodeStatus, 0, len(nodes))
	vpsMu.RLock()
	for _, n := range nodes {
		if st, ok := vpsStatuses[n.ID]; ok {
			out = append(out, st)
		} else {
			out = append(out, &VpsNodeStatus{ID: n.ID, Name: n.Name, Online: false, Error: "等待首次采集"})
		}
	}
	vpsMu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	c.JSON(http.StatusOK, gin.H{"success": true, "nodes": out})
}

type vpsNodeView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	BaseURL  string `json:"baseUrl"`
	UseProxy bool   `json:"useProxy"`
	HasKey   bool   `json:"hasKey"`
}

func VpsListNodes(c *gin.Context) {
	nodes := loadVpsNodes()
	views := make([]vpsNodeView, 0, len(nodes))
	for _, n := range nodes {
		views = append(views, vpsNodeView{ID: n.ID, Name: n.Name, BaseURL: n.BaseURL, UseProxy: n.UseProxy, HasKey: n.APIKey != ""})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "nodes": views})
}

func VpsSaveNodes(c *gin.Context) {
	var body struct {
		Nodes []VpsNode `json:"nodes"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "参数错误"})
		return
	}
	old := map[string]VpsNode{}
	for _, n := range loadVpsNodes() {
		old[n.ID] = n
	}
	out := make([]VpsNode, 0, len(body.Nodes))
	for _, n := range body.Nodes {
		n.BaseURL = normalizePanelBase(n.BaseURL)
		n.Name = strings.TrimSpace(n.Name)
		if n.Name == "" || n.BaseURL == "" {
			continue
		}
		if n.APIKey == "" {
			if o, ok := old[n.ID]; ok {
				n.APIKey = o.APIKey // 编辑时密钥留空 = 不修改
			}
		}
		if n.APIKey == "" {
			continue // 没有密钥的节点没法采集，跳过
		}
		out = append(out, n)
	}
	if err := saveVpsNodes(out); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "count": len(out)})
}

func VpsDeleteNode(c *gin.Context) {
	var body struct {
		ID string `json:"id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.ID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "参数错误"})
		return
	}
	nodes := loadVpsNodes()
	out := nodes[:0]
	for _, n := range nodes {
		if n.ID != body.ID {
			out = append(out, n)
		}
	}
	if err := saveVpsNodes(out); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	vpsMu.Lock()
	delete(vpsStatuses, body.ID)
	delete(vpsNetPrev, body.ID)
	vpsMu.Unlock()
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func VpsTestNode(c *gin.Context) {
	var body struct {
		ID       string `json:"id"`
		BaseURL  string `json:"baseUrl"`
		APIKey   string `json:"apiKey"`
		UseProxy bool   `json:"useProxy"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "参数错误"})
		return
	}
	key := body.APIKey
	if key == "" && body.ID != "" {
		for _, n := range loadVpsNodes() {
			if n.ID == body.ID {
				key = n.APIKey
			}
		}
	}
	node := VpsNode{ID: "test", Name: "test", BaseURL: normalizePanelBase(body.BaseURL), APIKey: key, UseProxy: body.UseProxy}
	if node.BaseURL == "" || key == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "error": "地址或密钥为空"})
		return
	}
	st := fetchNodeStatus(node, nil)
	if !st.Online {
		c.JSON(http.StatusOK, gin.H{"success": false, "error": st.Error})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true, "latencyMs": st.LatencyMs, "os": st.OS,
		"cpuPercent": st.CPUPercent, "memPercent": st.MemPercent,
		"containers": st.ContainersTotal,
	})
}

// ---- SSH 爆破日志（经 1Panel 文件接口读取系统 auth.log） ----
// 注意：新系统若只有 journald、没装 rsyslog，就没有 /var/log/auth.log 文件，
// 此时读不到属正常（SSHLogAvailable=false），装上 rsyslog 后自动生效。

type sshEntry struct {
	at     time.Time
	failed int
	lastIP string
	ok     bool
}

var vpsSSHCache = map[string]sshEntry{}

func sshDue(id string) bool {
	vpsMu.RLock()
	e, ok := vpsSSHCache[id]
	vpsMu.RUnlock()
	return !ok || time.Since(e.at) > 4*time.Minute
}

var (
	sshIPRe   = regexp.MustCompile(`from (\d+\.\d+\.\d+\.\d+)`)
	sshTimeRe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})[T ](\d{2}:\d{2}:\d{2})`)
	sshSysRe  = regexp.MustCompile(`^(\w{3})\s+(\d{1,2}) (\d{2}:\d{2}:\d{2})`)
)

func parseSSHLineTime(line string, now time.Time) (time.Time, bool) {
	if m := sshTimeRe.FindStringSubmatch(line); m != nil {
		t, err := time.ParseInLocation("2006-01-02 15:04:05", m[1]+" "+m[2], now.Location())
		return t, err == nil
	}
	if m := sshSysRe.FindStringSubmatch(line); m != nil {
		t, err := time.ParseInLocation("Jan 2 15:04:05 2006", fmt.Sprintf("%s %s %s %d", m[1], m[2], m[3], now.Year()), now.Location())
		if err == nil {
			if t.After(now.Add(24 * time.Hour)) {
				t = t.AddDate(-1, 0, 0) // 年初读到去年底的日志
			}
			return t, true
		}
	}
	return time.Time{}, false
}

func fetchSSHLog(node VpsNode) (int, string, bool) {
	for _, path := range []string{"/var/log/auth.log", "/var/log/secure"} {
		_, raw, _, err := panelCallAuto(node, "POST", "/api/v2/files/check", map[string]any{"path": path})
		if err != nil {
			continue
		}
		var chk panelResp
		if json.Unmarshal(raw, &chk) != nil || chk.Code != 200 || string(chk.Data) != "true" {
			continue
		}
		_, raw2, _, err := panelCallAuto(node, "POST", "/api/v2/files/content", map[string]any{"path": path})
		if err != nil {
			continue
		}
		var rd panelResp
		if json.Unmarshal(raw2, &rd) != nil || rd.Code != 200 {
			continue
		}
		var content string
		var fileData struct {
			Content string `json:"content"`
		}
		if json.Unmarshal(rd.Data, &fileData) == nil && fileData.Content != "" {
			content = fileData.Content
		} else if json.Unmarshal(rd.Data, &content) != nil {
			continue
		}
		if len(content) > 3<<20 {
			content = content[len(content)-(3<<20):]
		}
		now := time.Now()
		count := 0
		lastIP := ""
		for _, line := range strings.Split(content, "\n") {
			if !strings.Contains(line, "Failed password") {
				continue
			}
			if t, ok := parseSSHLineTime(line, now); ok && now.Sub(t) > 24*time.Hour {
				continue
			}
			count++
			if m := sshIPRe.FindStringSubmatch(line); m != nil {
				lastIP = m[1]
			}
		}
		return count, lastIP, true
	}
	return 0, "", false
}
