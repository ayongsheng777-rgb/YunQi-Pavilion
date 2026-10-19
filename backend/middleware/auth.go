package middleware

import (
	"flatnasgo-backend/config"
	"flatnasgo-backend/yunqi"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func parseToken(c *gin.Context) (*jwt.Token, error) {
	tokenString := c.GetHeader("Authorization")
	if tokenString == "" {
		tokenString = c.Query("token")
	} else {
		tokenString = strings.TrimPrefix(tokenString, "Bearer ")
	}

	if tokenString == "" {
		return nil, nil
	}

	return jwt.Parse(
		tokenString,
		func(token *jwt.Token) (interface{}, error) {
			return []byte(config.GetSecretKeyString()), nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
	)
}

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		username, ok := usernameFromJWT(c)

		// JWT 不行时，试试 30 天可信设备 cookie
		if !ok {
			if u, ok2 := yunqi.CheckDeviceCookie(c); ok2 {
				username, ok = u, true
			}
		}

		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录或登录已过期"})
			return
		}
		c.Set("username", username)
		c.Next()
	}
}

// usernameFromJWT 从请求里解析登录 JWT 的用户名。
// OTP 二次验证用的临时 token（含 otp_pending）不能当登录凭证，视为无效。
func usernameFromJWT(c *gin.Context) (string, bool) {
	token, err := parseToken(c)
	if err != nil || token == nil || !token.Valid {
		return "", false
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", false
	}
	if pending, _ := claims["otp_pending"].(bool); pending {
		return "", false
	}
	username, _ := claims["username"].(string)
	if username == "" {
		return "", false
	}
	return username, true
}

func OptionalAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := parseToken(c)

		if err == nil && token != nil && token.Valid {
			if claims, ok := token.Claims.(jwt.MapClaims); ok {
				c.Set("username", claims["username"])
			}
		}
		c.Next()
	}
}
