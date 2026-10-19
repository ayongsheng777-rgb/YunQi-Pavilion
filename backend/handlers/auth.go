package handlers

import (
	"flatnasgo-backend/config"
	"flatnasgo-backend/models"
	"flatnasgo-backend/utils"
	"flatnasgo-backend/yunqi"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

func Login(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式不对"})
		return
	}

	sysConfig := getCachedSystemConfig()

	if sysConfig.AuthMode == "single" && req.Username == "" {
		req.Username = "admin"
	}
	if sysConfig.AuthMode == "multi" && req.Username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名不能为空"})
		return
	}

	userFile := filepath.Join(config.UsersDir, req.Username+".json")
	if req.Username == "admin" && sysConfig.AuthMode == "single" {
		// Single mode admin data is in data.json
		userFile = filepath.Join(config.DataDir, "data.json")
	}

	var user models.User
	match := false

	if err := utils.ReadJSON(userFile, &user); err != nil {
		// If admin user not found, create default admin
		if req.Username == "admin" {
			hashed, err := bcrypt.GenerateFromPassword([]byte("admin"), 10)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "系统开小差了，请稍后再试"})
				return
			}
			user = models.User{
				Username: "admin",
				Password: string(hashed),
			}
			// Ensure directory exists
			if err := utils.WriteJSON(userFile, user); err == nil {
				// Successfully created default admin, now check password
				err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password))
				if err == nil {
					match = true
				}
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "创建默认账号失败"})
				return
			}
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码不正确"})
			return
		}
	} else {
		// User exists logic...
		storedPwd := user.Password
		if storedPwd == "" {
			storedPwd = "admin"
		}

		if len(storedPwd) > 0 && storedPwd[0] == '$' {
			err := bcrypt.CompareHashAndPassword([]byte(storedPwd), []byte(req.Password))
			if err == nil {
				match = true
			}
		} else {
			if req.Password == storedPwd {
				match = true
				hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), 10)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "系统开小差了，请稍后再试"})
					return
				}
				user.Password = string(hashed)
				if err := utils.WriteJSON(userFile, user); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败，请稍后再试"})
					return
				}
			}
		}
	}

	if match {
		// 云栖阁：该用户开启了 OTP 双因素验证时，不直接发登录 JWT，
		// 而是返回临时 token，前端拿它调 /api/login/otp 做二次验证。
		if yunqi.OTPEnabled(req.Username) {
			tmpToken, err := yunqi.IssueOTPToken(req.Username)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "生成验证凭证失败"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"need_otp": true, "tmp_token": tmpToken, "username": req.Username})
			return
		}
		// 原流程：直接签发 30 天 JWT（签发逻辑收拢到 yunqi.IssueUserToken，行为与原来一致）
		tokenString, err := yunqi.IssueUserToken(req.Username)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "签发登录凭证失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "token": tokenString, "username": req.Username})
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "密码不正确"})
	}
}

type AddUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LicenseRequest struct {
	Key string `json:"key"`
}

func initUserFile(username string, hashedPassword string) error {
	userFile := filepath.Join(config.UsersDir, username+".json")

	if _, err := os.Stat(userFile); err == nil {
		return nil
	}

	userData := map[string]interface{}{
		"username":      username,
		"password":      hashedPassword,
		"groups":        []interface{}{},
		"widgets":       []interface{}{},
		"appConfig":     map[string]interface{}{},
		"rssFeeds":      []interface{}{},
		"rssCategories": []interface{}{},
		"version":       int64(0),
	}

	var defaultData map[string]interface{}
	if err := utils.ReadJSON(config.DefaultFile, &defaultData); err == nil {
		for _, key := range []string{"groups", "widgets", "appConfig", "rssFeeds", "rssCategories"} {
			if v, ok := defaultData[key]; ok {
				userData[key] = v
			}
		}
	}

	return utils.WriteJSON(userFile, userData)
}

func Register(c *gin.Context) {
	sysConfig := getCachedSystemConfig()
	if sysConfig.AuthMode != "multi" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Registration is only available in multi-user mode"})
		return
	}

	var req AddUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	if req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Username and password required"})
		return
	}

	if len(req.Password) < 4 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Password must be at least 4 characters"})
		return
	}

	if req.Username == "admin" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot register as admin"})
		return
	}

	userFile := filepath.Join(config.UsersDir, req.Username+".json")
	if _, err := os.Stat(userFile); err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "User already exists"})
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), 10)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	if err := initUserFile(req.Username, string(hashed)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func GetUsers(c *gin.Context) {
	username := c.GetString("username")
	if username != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Permission denied"})
		return
	}

	files, err := os.ReadDir(config.UsersDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read users directory"})
		return
	}

	var users []string
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			name := strings.TrimSuffix(file.Name(), ".json")
			users = append(users, name)
		}
	}
	c.JSON(http.StatusOK, gin.H{"users": users})
}

func AddUser(c *gin.Context) {
	var req AddUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	if req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Username and password required"})
		return
	}

	if req.Username == "admin" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot add admin user manually"})
		return
	}

	userFile := filepath.Join(config.UsersDir, req.Username+".json")
	if _, err := os.Stat(userFile); err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "User already exists"})
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), 10)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	if err := initUserFile(req.Username, string(hashed)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func DeleteUser(c *gin.Context) {
	currentUsername := c.GetString("username")
	if currentUsername != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Permission denied"})
		return
	}

	username := c.Param("usr")
	if username == "" || username == "admin" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid username"})
		return
	}

	userFile := filepath.Join(config.UsersDir, username+".json")
	if err := os.Remove(userFile); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user"})
		return
	}

	cleanUserData(username)

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func cleanUserData(username string) {
	memoPattern := filepath.Join(config.DataDir, "memo_"+sanitizeFileName(username)+"_*.json")
	if matches, err := filepath.Glob(memoPattern); err == nil {
		for _, m := range matches {
			_ = os.Remove(m)
		}
	}

	transferUserDir := filepath.Join(config.DocDir, "transfer", "users", username)
	_ = os.RemoveAll(transferUserDir)

	_ = filepath.WalkDir(config.ConfigVersionsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasPrefix(d.Name(), sanitizeFileName(username)+"_") {
			_ = os.Remove(path)
		}
		return nil
	})
}

func sanitizeFileName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	s := strings.Trim(b.String(), "_")
	if s == "" {
		return "user"
	}
	return s
}

func UploadLicense(c *gin.Context) {
	var req LicenseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	licenseFile := filepath.Join(config.DataDir, "license.key")
	if err := os.WriteFile(licenseFile, []byte(req.Key), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save license"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
