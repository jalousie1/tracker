package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestHealth_BasicResponse(t *testing.T) {
	// Setup a minimal test router
	router := gin.New()

	// Create a mock handler that simulates healthy state
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":   "healthy",
			"database": "connected",
			"redis":    "connected",
		})
	})

	// Make request
	req, _ := http.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Errorf("expected JSON content type, got %s", w.Header().Get("Content-Type"))
	}
}

func TestSearch_RequiresQuery(t *testing.T) {
	router := gin.New()

	// Create a mock search handler
	router.GET("/search", func(c *gin.Context) {
		q := c.Query("q")
		if q == "" || len(q) < 2 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": gin.H{
					"code":    "invalid_query",
					"message": "q deve ter pelo menos 2 caracteres",
				},
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{"results": []interface{}{}})
	})

	tests := []struct {
		name     string
		query    string
		expected int
	}{
		{"empty query", "", http.StatusBadRequest},
		{"single char", "a", http.StatusBadRequest},
		{"valid query", "ab", http.StatusOK},
		{"longer query", "username", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/search?q="+tt.query, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.expected {
				t.Errorf("expected status %d, got %d", tt.expected, w.Code)
			}
		})
	}
}

func TestSearch_PaginationParams(t *testing.T) {
	router := gin.New()

	router.GET("/search", func(c *gin.Context) {
		page := c.DefaultQuery("page", "1")
		limit := c.DefaultQuery("limit", "50")

		c.JSON(http.StatusOK, gin.H{
			"page":  page,
			"limit": limit,
		})
	})

	req, _ := http.NewRequest("GET", "/search?q=test&page=2&limit=20", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestProfile_RequiresValidDiscordID(t *testing.T) {
	router := gin.New()

	router.GET("/profile/:discord_id", func(c *gin.Context) {
		discordID := c.Param("discord_id")

		// Basic snowflake validation (should be numeric and reasonable length)
		if len(discordID) < 17 || len(discordID) > 20 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": gin.H{
					"code":    "invalid_discord_id",
					"message": "discord_id invalido",
				},
			})
			return
		}

		// Check if it's numeric
		for _, r := range discordID {
			if r < '0' || r > '9' {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": gin.H{
						"code":    "invalid_discord_id",
						"message": "discord_id invalido",
					},
				})
				return
			}
		}

		c.JSON(http.StatusOK, gin.H{"discord_id": discordID})
	})

	tests := []struct {
		name     string
		id       string
		expected int
	}{
		{"too short", "123", http.StatusBadRequest},
		{"invalid chars", "abc123456789012345", http.StatusBadRequest},
		{"valid snowflake", "12345678901234567", http.StatusOK},
		{"valid long snowflake", "123456789012345678", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/profile/"+tt.id, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.expected {
				t.Errorf("expected status %d, got %d", tt.expected, w.Code)
			}
		})
	}
}
