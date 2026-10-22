package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"flatnasgo-backend/config"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/proxy"
)

func shouldDropProxyResponseHeader(header string) bool {
	switch strings.ToLower(strings.TrimSpace(header)) {
	case "content-security-policy",
		"content-security-policy-report-only",
		"x-frame-options",
		"x-content-security-policy",
		"x-webkit-csp",
		"frame-options",
		"cross-origin-embedder-policy",
		"cross-origin-opener-policy",
		"cross-origin-resource-policy",
		"permissions-policy",
		"clear-site-data",
		"report-to",
		"nel":
		return true
	default:
		return false
	}
}

func isAllowedWallpaperHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return false
	}
	raw := strings.TrimSpace(os.Getenv("WALLPAPER_WHITELIST"))
	presets := []string{"bing.biturl.top", "picsum.photos", "www.loliapi.com", "loliapi.com"}
	list := make([]string, 0, len(presets))
	list = append(list, presets...)
	if raw != "" {
		for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' || r == ';' }) {
			v := strings.TrimSpace(strings.ToLower(part))
			if v != "" {
				list = append(list, v)
			}
		}
	}
	for _, p := range list {
		if p == "" {
			continue
		}
		if host == p {
			return true
		}
		if strings.HasSuffix(host, "."+p) {
			return true
		}
		if strings.HasPrefix(host, p) {
			return true
		}
	}
	return false
}

func ProxyWallpaper(c *gin.Context) {
	targetURL := c.Query("url")
	requestUUID := c.Query("uuid")

	if targetURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "URL is required"})
		return
	}

	parsed, err := url.Parse(targetURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid URL"})
		return
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported protocol"})
		return
	}
	h := parsed.Hostname()
	if IsBlockedHost(h) && !isAllowedWallpaperHost(h) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Target host is not allowed"})
		return
	}

	req, err := http.NewRequest("GET", parsed.String(), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}

	// Forward necessary headers? Or just simple GET.
	// User-Agent might be needed for some APIs
	req.Header.Set("User-Agent", "FlatNas/1.0")

	// Reuse shared client or use a dedicated global one if needed.
	// For now, let's use the shared proxy client to support environments behind proxy.
	client, err := getSharedProxyClient()
	if err != nil {
		// Fallback to direct client if proxy setup fails, though unlikely
		client = &http.Client{Timeout: 10 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to fetch upstream URL"})
		return
	}
	defer resp.Body.Close()

	// Copy headers
	c.Header("Content-Type", resp.Header.Get("Content-Type"))
	if cc := resp.Header.Get("Cache-Control"); cc != "" {
		c.Header("Cache-Control", cc)
	}
	if etag := resp.Header.Get("ETag"); etag != "" {
		c.Header("ETag", etag)
	}

	// Set UUID if provided
	if requestUUID != "" {
		c.Header("X-Request-UUID", requestUUID)
	}

	c.Status(resp.StatusCode)
	_, err = io.Copy(c.Writer, resp.Body)
	if err != nil {
		fmt.Printf("Error streaming response: %v\n", err)
	}
}

func GetProxyStatus(c *gin.Context) {
	proxyURL, err := getProxyURL()
	if err != nil || proxyURL == nil {
		c.JSON(http.StatusOK, gin.H{"available": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"available": true})
}

func ProxyRequest(c *gin.Context) {
	targetURL := c.Query("url")
	if targetURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "URL is required"})
		return
	}
	parsed, err := url.Parse(targetURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid URL"})
		return
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported protocol"})
		return
	}
	if IsBlockedHost(parsed.Hostname()) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Target host is not allowed"})
		return
	}

	method := c.Request.Method
	if strings.EqualFold(method, "CONNECT") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported method"})
		return
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), method, parsed.String(), c.Request.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}
	for k, v := range c.Request.Header {
		key := http.CanonicalHeaderKey(k)
		if key == "Host" || key == "Content-Length" {
			continue
		}
		for _, vv := range v {
			req.Header.Add(k, vv)
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "FlatNas/1.0")
	}

	client, err := buildProxyClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Proxy unavailable"})
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to fetch upstream URL"})
		return
	}
	defer resp.Body.Close()

	for k, v := range resp.Header {
		if shouldDropProxyResponseHeader(k) {
			continue
		}
		for _, vv := range v {
			c.Header(k, vv)
		}
	}
	c.Header("X-Frame-Options", "")
	c.Header("Content-Security-Policy", "")
	c.Header("Content-Security-Policy-Report-Only", "")
	c.Header("Cross-Origin-Opener-Policy", "unsafe-none")
	c.Header("Cross-Origin-Embedder-Policy", "unsafe-none")
	c.Header("Cross-Origin-Resource-Policy", "cross-origin")
	c.Status(resp.StatusCode)
	_, err = io.Copy(c.Writer, resp.Body)
	if err != nil {
		fmt.Printf("Error streaming response: %v\n", err)
	}
}

func getProxyURL() (*url.URL, error) {
	keys := []string{"PROXY_URL", "HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"}
	var lastErr error
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}
		parsed, err := parseProxyURL(value)
		if err != nil {
			lastErr = err
			continue
		}
		return parsed, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, nil
}

func parseProxyURL(value string) (*url.URL, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return nil, fmt.Errorf("empty proxy url")
	}
	if !strings.Contains(normalized, "://") {
		normalized = "http://" + normalized
	}
	parsed, err := url.Parse(normalized)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid proxy url")
	}
	switch parsed.Scheme {
	case "http", "https", "socks5", "socks5h":
		return parsed, nil
	default:
		return nil, fmt.Errorf("unsupported proxy protocol")
	}
}

var (
	globalProxyClient *http.Client
	globalProxyURLStr string
	proxyClientMu     sync.RWMutex
)

func getSharedProxyClient() (*http.Client, error) {
	proxyURL, err := getProxyURL()
	if err != nil {
		return nil, err
	}
	if proxyURL == nil {
		proxyURL, _ = fileProxyURL()
	}

	currentURLStr := ""
	if proxyURL != nil {
		currentURLStr = proxyURL.String()
	}

	proxyClientMu.RLock()
	client := globalProxyClient
	cachedURLStr := globalProxyURLStr
	proxyClientMu.RUnlock()

	if client != nil && cachedURLStr == currentURLStr {
		return client, nil
	}

	proxyClientMu.Lock()
	defer proxyClientMu.Unlock()

	// Double check
	if globalProxyClient != nil && globalProxyURLStr == currentURLStr {
		return globalProxyClient, nil
	}

	// Rebuild client
	direct := &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		DialContext:           (&net.Dialer{Timeout: 6 * time.Second}).DialContext,
		TLSHandshakeTimeout:   6 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}

	if proxyURL == nil {
		newClient := &http.Client{Timeout: 20 * time.Second, Transport: direct}
		globalProxyClient = newClient
		globalProxyURLStr = ""
		return newClient, nil
	}

	proxyTr := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
	switch proxyURL.Scheme {
	case "http", "https":
		proxyTr.Proxy = http.ProxyURL(proxyURL)
	case "socks5", "socks5h":
		dialer, err := proxy.FromURL(proxyURL, proxy.Direct)
		if err != nil {
			return nil, err
		}
		proxyTr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		}
	default:
		return nil, fmt.Errorf("unsupported proxy protocol")
	}

	// 直连优先、代理兜底：国内接口（壁纸 API、必应等）直连最稳，
	// 直连失败或被挡（>=400）才走代理，避免"开了代理反而取不到"。
	newClient := &http.Client{Timeout: 25 * time.Second, Transport: &fallbackTransport{direct: direct, proxy: proxyTr}}
	globalProxyClient = newClient
	globalProxyURLStr = currentURLStr
	return newClient, nil
}

// fileProxyURL 从云栖阁配置（data/yunqi/vps_nodes.json）读带账号的代理地址，
// 作为环境变量没设时的兜底代理来源（sing-box 2026-10-21 起需 anyong 认证）。
func fileProxyURL() (*url.URL, error) {
	if config.DataDir == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(filepath.Join(config.DataDir, "yunqi", "vps_nodes.json"))
	if err != nil {
		return nil, nil
	}
	var cfg struct {
		ProxyURL string `json:"proxyUrl"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil || cfg.ProxyURL == "" {
		return nil, nil
	}
	return parseProxyURL(cfg.ProxyURL)
}

// fallbackTransport 先直连；失败或状态 >=400 且请求可重放时改走代理重试一次。
type fallbackTransport struct {
	direct *http.Transport
	proxy  *http.Transport
}

func (t *fallbackTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.direct.RoundTrip(req)
	if err == nil && resp != nil && resp.StatusCode < 400 {
		return resp, nil
	}
	if t.proxy == nil {
		return resp, err
	}
	// 只有可重放的请求才走代理重试（GET/HEAD，或带 GetBody 的请求）
	if req.Method != http.MethodGet && req.Method != http.MethodHead && req.GetBody == nil {
		return resp, err
	}
	if err == nil && resp != nil {
		// 直连拿到了 >=400：先把错误页正文存下来，代理也失败时还能原样返回
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(buf))
	}
	req2 := req.Clone(req.Context())
	if req.GetBody != nil {
		if b, e := req.GetBody(); e == nil {
			req2.Body = b
		}
	}
	resp2, err2 := t.proxy.RoundTrip(req2)
	if err2 != nil {
		return resp, err
	}
	return resp2, nil
}

func buildProxyClient() (*http.Client, error) {
	return getSharedProxyClient()
}

func IsBlockedHost(host string) bool {
	if host == "" {
		return true
	}
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "localhost" || host == "localhost." {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return isBlockedIP(ip)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return true
	}
	for _, item := range ips {
		if item.IP != nil && isBlockedIP(item.IP) {
			return true
		}
	}
	return false
}

func isBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}
