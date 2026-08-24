package middleware_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"obsidian/internal/api/middleware"

	"github.com/golang-jwt/jwt/v5"
)

func generateTestToken(secret []byte, userID, role string, exp time.Time) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userId": userID,
		"role":   role,
		"exp":    exp.Unix(),
	})
	tokenStr, _ := token.SignedString(secret)
	return tokenStr
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	middleware.InitJWTSecret("test-secret-key-12345")

	var capturedUserID, capturedRole string
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUserID = r.Context().Value(middleware.UserIDKey).(string)
		capturedRole = r.Context().Value(middleware.UserRoleKey).(string)
		w.WriteHeader(http.StatusOK)
	})

	token := generateTestToken(middleware.JWTSecret, "user-42", "admin", time.Now().Add(1*time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	middleware.Auth(nextHandler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if capturedUserID != "user-42" {
		t.Errorf("expected userID 'user-42', got %q", capturedUserID)
	}
	if capturedRole != "admin" {
		t.Errorf("expected role 'admin', got %q", capturedRole)
	}
}

func TestAuthMiddleware_MissingHeader(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	rec := httptest.NewRecorder()

	middleware.Auth(nextHandler).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 Unauthorized, got %d", rec.Code)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	middleware.InitJWTSecret("test-secret-key-12345")

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	req.Header.Set("Authorization", "Bearer invalid.jwt.token")
	rec := httptest.NewRecorder()

	middleware.Auth(nextHandler).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 Unauthorized on bad token, got %d", rec.Code)
	}
}

func TestAuthMiddleware_WebSocketQueryToken(t *testing.T) {
	middleware.InitJWTSecret("test-secret-key-12345")

	var capturedUserID string
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUserID = r.Context().Value(middleware.UserIDKey).(string)
		w.WriteHeader(http.StatusOK)
	})

	token := generateTestToken(middleware.JWTSecret, "ws-user-99", "member", time.Now().Add(1*time.Hour))

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/ws?token=%s", token), nil)
	rec := httptest.NewRecorder()

	middleware.Auth(nextHandler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 via query param token, got %d", rec.Code)
	}
	if capturedUserID != "ws-user-99" {
		t.Errorf("expected userID 'ws-user-99', got %q", capturedUserID)
	}
}

func TestRequireRole_AccessControl(t *testing.T) {
	middleware.InitJWTSecret("test-secret-key-12345")

	adminHandler := middleware.RequireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("admin-success"))
	}))

	// Case 1: Member trying to access admin endpoint -> 403 Forbidden
	memberToken := generateTestToken(middleware.JWTSecret, "member-1", "member", time.Now().Add(1*time.Hour))
	req1 := httptest.NewRequest(http.MethodPut, "/api/queues/123", nil)
	req1.Header.Set("Authorization", "Bearer "+memberToken)
	rec1 := httptest.NewRecorder()
	middleware.Auth(adminHandler).ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for member on admin route, got %d", rec1.Code)
	}

	// Case 2: Admin accessing admin endpoint -> 200 OK
	adminToken := generateTestToken(middleware.JWTSecret, "admin-1", "admin", time.Now().Add(1*time.Hour))
	req2 := httptest.NewRequest(http.MethodPut, "/api/queues/123", nil)
	req2.Header.Set("Authorization", "Bearer "+adminToken)
	rec2 := httptest.NewRecorder()
	middleware.Auth(adminHandler).ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("expected 200 OK for admin on admin route, got %d", rec2.Code)
	}
}

func TestCORSMiddleware(t *testing.T) {
	handler := middleware.CORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Options preflight
	req := httptest.NewRequest(http.MethodOptions, "/api/projects", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 on OPTIONS preflight, got %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("missing CORS Allow-Origin header")
	}
}

func TestLoggerMiddleware(t *testing.T) {
	handler := middleware.Logger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test-log", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", rec.Code)
	}
}
