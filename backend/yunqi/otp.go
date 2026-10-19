package yunqi

import (
	"crypto/sha256"
	"encoding/hex"
	"flatnasgo-backend/config"
	"flatnasgo-backend/utils"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// ---------------------------------------------------------------------------
// 模块 5：OTP 双因素 + 30 天可信设备
// OTP secret 存 config.DataDir/yunqi/otp.json：{username: {"secret","enabled"}}
// 可信设备存 config.DataDir/yunqi/devices.json（只存 token 的 SHA256）。
// ---------------------------------------------------------------------------

const (
	otpIssuer       = "云栖阁"
	deviceCookie    = "yq_device"
	deviceMaxAge    = 30 * 24 * 3600 // 30 天
	tmpTokenMinutes = 5
)

type otpRecord struct {
	Secret  string `json:"secret"`
	Enabled bool   `json:"enabled"`
}

var otpMu sync.Mutex

func otpFile() string { return filepath.Join(YunqiDir(), "otp.json") }

func loadOTPStore() map[string]otpRecord {
	otpMu.Lock()
	defer otpMu.Unlock()
	var st map[string]otpRecord
	_ = utils.ReadJSON(otpFile(), &st)
	if st == nil {
		st = map[string]otpRecord{}
	}
	return st
}

func saveOTPStore(st map[string]otpRecord) error {
	otpMu.Lock()
	defer otpMu.Unlock()
	return utils.WriteJSON(otpFile(), st)
}

// OTPEnabled 某用户是否启用了 OTP（供 handlers.Login 调用）。
func OTPEnabled(username string) bool {
	st := loadOTPStore()
	rec, ok := st[username]
	return ok && rec.Enabled && strings.TrimSpace(rec.Secret) != ""
}

func otpSecretOf(username string) string {
	st := loadOTPStore()
	rec, ok := st[username]
	if !ok || !rec.Enabled {
		return ""
	}
	return rec.Secret
}

func verifyTOTP(secret, code string) bool {
	code = strings.TrimSpace(code)
	if secret == "" || code == "" {
		return false
	}
	ok, err := totp.ValidateCustom(code, secret, time.Now(), totp.ValidateOpts{
		Period:    30,
		Skew:      1, // 允许 ±1 个时间窗漂移
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	return err == nil && ok
}

// POST /api/auth/otp/setup（JWT）-> {"secret","otpauth_url"}（secret 先存为 enabled=false）
func OTPSetup(c *gin.Context) {
	username := c.GetString("username")
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      otpIssuer,
		AccountName: username,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成密钥失败，请稍后再试"})
		return
	}
	st := loadOTPStore()
	st[username] = otpRecord{Secret: key.Secret(), Enabled: false}
	if err := saveOTPStore(st); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败，请稍后再试"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"secret": key.Secret(), "otpauth_url": key.URL()})
}

// POST /api/auth/otp/enable（JWT）{"code"} -> 校验通过则 enabled=true
func OTPEnable(c *gin.Context) {
	username := c.GetString("username")
	var req struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式不对"})
		return
	}
	st := loadOTPStore()
	rec, ok := st[username]
	if !ok || strings.TrimSpace(rec.Secret) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先扫码绑定验证器"})
		return
	}
	if !verifyTOTP(rec.Secret, req.Code) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "验证码不对，请对一下手机上的 6 位数字再试"})
		return
	}
	rec.Enabled = true
	st[username] = rec
	if err := saveOTPStore(st); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败，请稍后再试"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GET /api/auth/otp/status（JWT）-> {"enabled":bool} 前端用它显示精确的绑定状态
func OTPStatus(c *gin.Context) {
	username := c.GetString("username")
	c.JSON(http.StatusOK, gin.H{"enabled": OTPEnabled(username)})
}

// POST /api/auth/otp/disable（JWT）{"code"} -> 校验通过则删除该用户记录
func OTPDisable(c *gin.Context) {
	username := c.GetString("username")
	var req struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式不对"})
		return
	}
	st := loadOTPStore()
	rec, ok := st[username]
	if !ok || !rec.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "你还没开启双因素验证"})
		return
	}
	if !verifyTOTP(rec.Secret, req.Code) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "验证码不对，请对一下手机上的 6 位数字再试"})
		return
	}
	delete(st, username)
	if err := saveOTPStore(st); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败，请稍后再试"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ---------------------------------------------------------------------------
// 临时 token（OTP 二次验证用，5 分钟有效，含 otp_pending=true）
// ---------------------------------------------------------------------------

// IssueOTPToken 签发 OTP 待验证的临时 token。
func IssueOTPToken(username string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username":    username,
		"otp_pending": true,
		"exp":         time.Now().Add(tmpTokenMinutes * time.Minute).Unix(),
	})
	return token.SignedString([]byte(config.GetSecretKeyString()))
}

// ParseOTPToken 校验临时 token，返回用户名。过期/伪造返回 ok=false。
func ParseOTPToken(tokenString string) (username string, ok bool) {
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		return []byte(config.GetSecretKeyString()), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || token == nil || !token.Valid {
		return "", false
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", false
	}
	if pending, _ := claims["otp_pending"].(bool); !pending {
		return "", false
	}
	username, _ = claims["username"].(string)
	if username == "" {
		return "", false
	}
	return username, true
}

// ---------------------------------------------------------------------------
// 可信设备（30 天）
// ---------------------------------------------------------------------------

type trustedDevice struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	TokenHash  string `json:"token_hash"` // token 的 SHA256 hex，只存哈希
	Name       string `json:"name"`
	CreatedAt  int64  `json:"created_at"`
	LastUsedAt int64  `json:"last_used_at"`
	ExpiresAt  int64  `json:"expires_at"`
}

var deviceMu sync.Mutex

func deviceFile() string { return filepath.Join(YunqiDir(), "devices.json") }

func loadDevices() []trustedDevice {
	deviceMu.Lock()
	defer deviceMu.Unlock()
	var devs []trustedDevice
	_ = utils.ReadJSON(deviceFile(), &devs)
	if devs == nil {
		devs = []trustedDevice{}
	}
	return devs
}

func saveDevices(devs []trustedDevice) error {
	deviceMu.Lock()
	defer deviceMu.Unlock()
	if devs == nil {
		devs = []trustedDevice{}
	}
	return utils.WriteJSON(deviceFile(), devs)
}

func hashDeviceToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// addTrustedDevice 新增可信设备，返回明文 token（只在此时可见一次）。
func addTrustedDevice(username, name string) (string, error) {
	token, err := randomHex(32) // 256 bit
	if err != nil {
		return "", err
	}
	now := time.Now().Unix()
	devs := loadDevices()
	devs = append(devs, trustedDevice{
		ID:         newUUID(),
		Username:   username,
		TokenHash:  hashDeviceToken(token),
		Name:       name,
		CreatedAt:  now,
		LastUsedAt: now,
		ExpiresAt:  now + deviceMaxAge,
	})
	if err := saveDevices(devs); err != nil {
		return "", err
	}
	return token, nil
}

// CheckDeviceCookie 校验 yq_device cookie：命中且未过期则返回用户名，并更新 last_used_at。
func CheckDeviceCookie(c *gin.Context) (string, bool) {
	token, err := c.Cookie(deviceCookie)
	if err != nil || strings.TrimSpace(token) == "" {
		return "", false
	}
	h := hashDeviceToken(strings.TrimSpace(token))
	devs := loadDevices()
	now := time.Now().Unix()
	for i := range devs {
		if devs[i].TokenHash != h {
			continue
		}
		if devs[i].ExpiresAt <= now {
			return "", false
		}
		devs[i].LastUsedAt = now
		_ = saveDevices(devs)
		return devs[i].Username, true
	}
	return "", false
}

func deviceNameFromRequest(c *gin.Context) string {
	ua := strings.TrimSpace(c.GetHeader("User-Agent"))
	if ua == "" {
		return "未知设备"
	}
	return truncateRunes(ua, 80)
}

// POST /api/login/otp（无需鉴权）{"tmp_token","code","remember_device"}
func LoginOTP(c *gin.Context) {
	var req struct {
		TmpToken       string `json:"tmp_token"`
		Code           string `json:"code"`
		RememberDevice bool   `json:"remember_device"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式不对"})
		return
	}
	username, ok := ParseOTPToken(strings.TrimSpace(req.TmpToken))
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "验证超时或凭证不对，请重新登录"})
		return
	}
	secret := otpSecretOf(username)
	if secret == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "这个账号没开双因素验证，请直接登录"})
		return
	}
	if !verifyTOTP(secret, req.Code) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "验证码不对，请对一下手机上的 6 位数字再试"})
		return
	}
	// 沿用 Login 原来的签发逻辑发正常 JWT
	tokenString, err := IssueUserToken(username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "签发登录凭证失败"})
		return
	}
	if req.RememberDevice {
		deviceToken, err := addTrustedDevice(username, deviceNameFromRequest(c))
		if err == nil {
			secure := c.Request.TLS != nil // 只有 HTTPS 才加 Secure
			c.SetSameSite(http.SameSiteLaxMode)
			c.SetCookie(deviceCookie, deviceToken, deviceMaxAge, "/", "", secure, true)
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "token": tokenString, "username": username})
}

// GET /api/auth/devices（JWT）列出自己的可信设备
func ListDevices(c *gin.Context) {
	username := c.GetString("username")
	devs := loadDevices()
	out := []gin.H{}
	for _, d := range devs {
		if d.Username != username {
			continue
		}
		out = append(out, gin.H{
			"id":           d.ID,
			"name":         d.Name,
			"created_at":   d.CreatedAt,
			"last_used_at": d.LastUsedAt,
			"expires_at":   d.ExpiresAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"devices": out})
}

// DELETE /api/auth/devices/:id（JWT）删除自己的可信设备
func DeleteDevice(c *gin.Context) {
	username := c.GetString("username")
	id := c.Param("id")
	devs := loadDevices()
	kept := make([]trustedDevice, 0, len(devs))
	found := false
	for _, d := range devs {
		if d.ID == id && d.Username == username {
			found = true
			continue
		}
		kept = append(kept, d)
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "找不到这个设备"})
		return
	}
	if err := saveDevices(kept); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败，请稍后再试"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GET /api/auth/me（无需鉴权）：JWT 或 device cookie 有效 -> 登录态
func AuthMe(c *gin.Context) {
	if username, ok := ParseUsername(bearerOrQueryToken(c.GetHeader("Authorization"), c.Query("token"))); ok {
		c.JSON(http.StatusOK, gin.H{"logged_in": true, "username": username})
		return
	}
	if username, ok := CheckDeviceCookie(c); ok {
		c.JSON(http.StatusOK, gin.H{"logged_in": true, "username": username})
		return
	}
	c.JSON(http.StatusOK, gin.H{"logged_in": false})
}
