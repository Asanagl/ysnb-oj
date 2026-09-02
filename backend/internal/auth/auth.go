// Package auth issues/validates JWTs, hashes passwords and provides the
// Gin middleware used by every protected route.
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/ysnb/oj/internal/model"
)

const (
	ClaimsKey = "auth.claims"
	RoleKey   = "auth.role"
	IDKey     = "auth.id"
)

type Claims struct {
	UserID   uint   `json:"uid"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// Manager signs and verifies user tokens; the secret comes from config only.
type Manager struct {
	secret []byte
	expire time.Duration
}

func NewManager(secret string, expireHours int) *Manager {
	return &Manager{secret: []byte(secret), expire: time.Duration(expireHours) * time.Hour}
}

func (m *Manager) Issue(u *model.User) (string, error) {
	claims := Claims{
		UserID:   u.ID,
		Username: u.Username,
		Role:     u.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.Username,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(m.expire)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

func (m *Manager) Parse(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			// why: algorithm confusion attacks swap HS256 for "none"/RS with
			// a public key we never configured; refuse anything but HMAC.
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		return m.secret, nil
	})
	if err != nil || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

// HashPassword / CheckPassword wrap bcrypt so handlers never touch raw bytes.
func HashPassword(plain string) (string, error) {
	hash, err := bcryptGenerateFromPassword(plain)
	return hash, err
}

func CheckPassword(hash, plain string) bool {
	return bcryptCompare(hash, plain)
}

// Middleware validates the Bearer token and stashes claims on the context.
func (m *Manager) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		if len(raw) > 7 && raw[:7] == "Bearer " {
			if claims, err := m.Parse(raw[7:]); err == nil {
				c.Set(ClaimsKey, claims)
				c.Set(RoleKey, claims.Role)
				c.Set(IDKey, claims.UserID)
			}
		}
		// No abort here: routes decide whether anonymity is acceptable.
		c.Next()
	}
}

// RequireAuth rejects anonymous requests.
func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := c.Get(ClaimsKey); !ok {
			c.AbortWithStatusJSON(401, gin.H{"error": "login required"})
			return
		}
		c.Next()
	}
}

// RequireRole rejects requests whose role is not in allowed.
func RequireRole(allowed ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get(RoleKey)
		roleStr, _ := role.(string)
		for _, r := range allowed {
			if r == roleStr {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(403, gin.H{"error": "insufficient permission"})
	}
}

// IsSuperAdmin: super_admin outranks admin — only this role may grant or
// revoke admin/super_admin grants and ban/unban accounts. super_admin also
// inherits every admin power (role checks treat it as admin).
func IsSuperAdmin(role string) bool { return role == model.RoleSuperAdmin }

// CurrentUser returns the authenticated claims, or nil when anonymous.
func CurrentUser(c *gin.Context) *Claims {
	v, ok := c.Get(ClaimsKey)
	if !ok {
		return nil
	}
	claims, _ := v.(*Claims)
	return claims
}
