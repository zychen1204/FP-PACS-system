package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// ── Issue ──────────────────────────────────────────────────────────────────

func TestIssue_ReturnsNonEmptyToken(t *testing.T) {
	tok, err := Issue("B001", time.Hour)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if tok == "" {
		t.Fatal("expected non-empty token string")
	}
}

// ── Parse ──────────────────────────────────────────────────────────────────

func TestParse_ValidToken_ReturnsBadgeID(t *testing.T) {
	tok, _ := Issue("B001", time.Hour)
	claims, err := Parse(tok)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if claims.BadgeID != "B001" {
		t.Errorf("badge_id got=%q want=B001", claims.BadgeID)
	}
}

func TestParse_ExpiredToken_ReturnsError(t *testing.T) {
	tok, _ := Issue("B001", -time.Second) // already expired at issue time
	if _, err := Parse(tok); err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestParse_WrongSignature_ReturnsError(t *testing.T) {
	t.Setenv("JWT_SECRET", "secret-a")
	tok, _ := Issue("B001", time.Hour)

	t.Setenv("JWT_SECRET", "secret-b") // switch secret before parsing
	if _, err := Parse(tok); err == nil {
		t.Fatal("expected signature mismatch error, got nil")
	}
}

func TestParse_MalformedToken_ReturnsError(t *testing.T) {
	if _, err := Parse("this.is.not.a.jwt"); err == nil {
		t.Fatal("expected error for malformed token, got nil")
	}
}

// ── Middleware ─────────────────────────────────────────────────────────────

func newTestRouter(mw gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/", mw, func(c *gin.Context) {
		id, _ := BadgeIDFromCtx(c)
		c.String(http.StatusOK, id)
	})
	return r
}

func TestMiddleware_ValidBearer_SetsBadgeID(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "0")
	tok, _ := Issue("B001", time.Hour)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	newTestRouter(Middleware()).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200", w.Code)
	}
	if w.Body.String() != "B001" {
		t.Errorf("body got=%q want=B001", w.Body.String())
	}
}

func TestMiddleware_MissingHeader_Returns401(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "0")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)
	newTestRouter(Middleware()).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status got=%d want=401", w.Code)
	}
}

func TestMiddleware_InvalidToken_Returns401(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "0")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer garbage.token.here")
	newTestRouter(Middleware()).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status got=%d want=401", w.Code)
	}
}

// FR-10: DEV_AUTH_BYPASS=1 bypasses JWT and reads badge_id from ?as=
func TestMiddleware_DevBypass_SetsQueryParam(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "1")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/?as=B100", nil)
	newTestRouter(Middleware()).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200", w.Code)
	}
	if w.Body.String() != "B100" {
		t.Errorf("body got=%q want=B100", w.Body.String())
	}
}

func TestMiddleware_DevBypass_DefaultBadgeID(t *testing.T) {
	t.Setenv("DEV_AUTH_BYPASS", "1")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil) // no ?as=
	newTestRouter(Middleware()).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status got=%d want=200", w.Code)
	}
	if w.Body.String() == "" {
		t.Error("expected a default badge_id, got empty string")
	}
}
