package auth

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// Claims is the JWT payload for FR-10 (reporting API auth).
// 設計上 caller 對自己持有的 badge_id 負責；reporting-api 在處理 FR-6/9
// 階層 scope 時，用 ctx.Get("badge_id") 取出後查 employees 取 manager scope。
type Claims struct {
	BadgeID string `json:"badge_id"`
	jwt.RegisteredClaims
}

// secret returns HS256 secret from env; falls back to dev default with a log warning.
// 正式環境必須設 JWT_SECRET。
func secret() []byte {
	s := os.Getenv("JWT_SECRET")
	if s == "" {
		// dev-only default — 生產務必覆蓋
		s = "dev-only-not-for-prod-change-me-please"
	}
	return []byte(s)
}

// Issue produces a signed JWT for the given badge_id with given TTL.
// 由 /v1/dev/login 呼叫（FR-10 demo IdP 模擬）。
func Issue(badgeID string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		BadgeID: badgeID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   badgeID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			Issuer:    "pacs-dev",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret())
}

// Parse validates & returns the claims. Returns error on signature / expiry failure.
func Parse(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret(), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

// Middleware validates Bearer token and exposes badge_id via gin Context.
// 在 reporting-api 上套用 /v1/reports/* 與 /v1/audit。
// dev bypass：若 env DEV_AUTH_BYPASS=1，跳過驗證並以 query ?as= 指定 badge_id。
func Middleware() gin.HandlerFunc {
	bypass := os.Getenv("DEV_AUTH_BYPASS") == "1"
	return func(c *gin.Context) {
		if bypass {
			as := c.Query("as")
			if as == "" {
				as = "B100" // dev default：fab manager 視角
			}
			c.Set("badge_id", as)
			c.Next()
			return
		}

		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing or malformed Authorization header",
			})
			return
		}
		tokenStr := strings.TrimPrefix(auth, "Bearer ")
		claims, err := Parse(tokenStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": fmt.Sprintf("invalid token: %v", err),
			})
			return
		}
		c.Set("badge_id", claims.BadgeID)
		c.Next()
	}
}

// BadgeIDFromCtx fetches the authenticated badge_id from gin context.
func BadgeIDFromCtx(c *gin.Context) (string, bool) {
	v, ok := c.Get("badge_id")
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}
