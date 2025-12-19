package api

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func (s *Server) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		allowed := false
		for _, allowedOrigin := range s.cfg.CORSOrigins {
			if origin == allowedOrigin || allowedOrigin == "*" {
				allowed = true
				break
			}
		}

		if allowed {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Admin-Key")
			c.Header("Access-Control-Max-Age", "3600")
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func (s *Server) loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		clientIP := c.ClientIP()

		s.log.Info("http_request",
			"method", method,
			"path", path,
			"status", status,
			"latency_ms", latency.Milliseconds(),
			"client_ip", clientIP,
		)
	}
}

func (s *Server) rateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		clientIP := c.ClientIP()
		path := c.Request.URL.Path

		// limites diferentes por endpoint
		var limit int64 = 60 // default: 60 req/min
		var window time.Duration = 1 * time.Minute

		if strings.HasPrefix(path, "/api/v1/search") {
			limit = 20
		} else if strings.HasPrefix(path, "/api/v1/admin") {
			limit = 10
		}

		// sliding window usando sorted set do redis
		now := time.Now().Unix()
		windowSeconds := int64(window.Seconds())
		key := fmt.Sprintf("ratelimit:sw:%s:%s", clientIP, path)

		ctx := c.Request.Context()

		// remover entradas antigas (fora da janela)
		oldest := now - windowSeconds
		_ = s.redis.RDB().ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", oldest)).Err()

		// contar requisições na janela
		count, err := s.redis.RDB().ZCard(ctx, key).Result()
		if err != nil {
			s.log.Warn("rate_limit_error", "error", err)
			c.Next()
			return
		}

		if count >= limit {
			// calcular retry after baseado na mais antiga requisição na janela
			oldestReq, _ := s.redis.RDB().ZRangeWithScores(ctx, key, 0, 0).Result()
			var retryAfter int64 = windowSeconds
			if len(oldestReq) > 0 {
				retryAfter = windowSeconds - (now - int64(oldestReq[0].Score))
				if retryAfter < 0 {
					retryAfter = 0
				}
			}

			c.Header("Retry-After", fmt.Sprintf("%d", retryAfter))
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{
					"code":    "rate_limited",
					"message": "too many requests",
				},
			})
			c.Abort()
			return
		}

		// adicionar requisição atual
		member := fmt.Sprintf("%d", now)
		_ = s.redis.RDB().ZAdd(ctx, key, redis.Z{
			Score:  float64(now),
			Member: member,
		}).Err()
		_ = s.redis.RDB().Expire(ctx, key, window).Err()

		c.Next()
	}
}

func (s *Server) inputValidationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// validar e sanitizar query parameters
		query := c.Request.URL.Query()
		for _, values := range query {
			for i, value := range values {
				// remover caracteres de controle e limitar tamanho
				sanitized := sanitizeInput(value)
				if len(sanitized) > 500 {
					c.JSON(http.StatusBadRequest, gin.H{
						"error": gin.H{
							"code":    "invalid_parameter",
							"message": "parametro muito longo",
						},
					})
					c.Abort()
					return
				}
				values[i] = sanitized
			}
		}

		// validar path parameters
		for _, param := range c.Params {
			if len(param.Value) > 100 {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": gin.H{
						"code":    "invalid_parameter",
						"message": "parametro muito longo",
					},
				})
				c.Abort()
				return
			}
			// sanitizar path parameter
			param.Value = sanitizeInput(param.Value)
		}

		c.Next()
	}
}

func sanitizeInput(input string) string {
	// remover caracteres de controle (exceto \n, \r, \t)
	result := make([]rune, 0, len(input))
	for _, r := range input {
		if r >= 32 || r == '\n' || r == '\r' || r == '\t' {
			result = append(result, r)
		}
	}
	return string(result)
}

func (s *Server) adminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// falha rapida se o backend nao foi configurado
		if strings.TrimSpace(s.cfg.AdminSecretKey) == "" {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{
					"code":    "config_error",
					"message": "ADMIN_SECRET_KEY nao configurada no backend",
				},
			})
			c.Abort()
			return
		}

		adminKey := strings.TrimSpace(c.GetHeader("X-Admin-Key"))
		if adminKey == "" {
			// compat: Authorization: Bearer <key>
			auth := strings.TrimSpace(c.GetHeader("Authorization"))
			if strings.HasPrefix(auth, "Bearer ") {
				adminKey = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
			}
		}
		if adminKey == "" {
			// compat: query param (debug)
			adminKey = strings.TrimSpace(c.Query("admin_key"))
		}
		if adminKey == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"code":    "unauthorized",
					"message": "missing admin key (use X-Admin-Key header)",
				},
			})
			c.Abort()
			return
		}

		// compare constante pra evitar timing leaks
		if subtle.ConstantTimeCompare([]byte(adminKey), []byte(s.cfg.AdminSecretKey)) != 1 {
			c.JSON(http.StatusForbidden, gin.H{
				"error": gin.H{
					"code":    "forbidden",
					"message": "invalid admin key",
				},
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

