package middleware

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
)

const maxLoggedBodyBytes = 2048

func Timeout(duration time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), duration)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = newRequestID()
		}
		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

func RequestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		requestID, _ := c.Get("request_id")
		requestBytes := c.Request.ContentLength
		if requestBytes < 0 {
			requestBytes = 0
		}
		logger.Info("HTTP started",
			"method", c.Request.Method,
			"uri", c.Request.URL.RequestURI(),
			"request_bytes", requestBytes,
			"request_id", requestID,
		)

		if c.Request.Body != nil {
			requestBody, err := io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewReader(requestBody))
			if err != nil {
				logger.Warn("Request Body: unable to read body", "error", err, "request_id", requestID)
			} else {
				if len(requestBody) > 0 {
					logger.Info("Request Body: "+formatLoggedBody(requestBody), "request_id", requestID)
				}
			}
		}

		responseWriter := &bodyLogWriter{ResponseWriter: c.Writer}
		c.Writer = responseWriter
		c.Next()
		logger.Info("HTTP completed",
			"method", c.Request.Method,
			"uri", c.Request.URL.RequestURI(),
			"status", c.Writer.Status(),
			"duration", time.Since(start),
			"request_bytes", requestBytes,
			"response_bytes", c.Writer.Size(),
			"request_id", requestID,
		)
		if responseWriter.total > 0 {
			logger.Info("Response Body: "+formatCapturedBody(responseWriter.body.Bytes(), responseWriter.total), "request_id", requestID)
		}
	}
}

type bodyLogWriter struct {
	gin.ResponseWriter
	body  bytes.Buffer
	total int
}

func (w *bodyLogWriter) Write(data []byte) (int, error) {
	w.capture(data)
	return w.ResponseWriter.Write(data)
}

func (w *bodyLogWriter) WriteString(data string) (int, error) {
	w.capture([]byte(data))
	return w.ResponseWriter.WriteString(data)
}

func (w *bodyLogWriter) capture(data []byte) {
	w.total += len(data)
	remaining := maxLoggedBodyBytes - w.body.Len()
	if remaining <= 0 {
		return
	}
	if len(data) > remaining {
		data = data[:remaining]
	}
	_, _ = w.body.Write(data)
}

func formatLoggedBody(body []byte) string {
	if len(body) <= maxLoggedBodyBytes {
		return string(body)
	}
	return formatCapturedBody(body[:maxLoggedBodyBytes], len(body))
}

func formatCapturedBody(body []byte, total int) string {
	if total <= maxLoggedBodyBytes {
		return string(body)
	}
	return fmt.Sprintf("%s... [truncated, showing %d of %d bytes]", body, maxLoggedBodyBytes, total)
}

func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				requestID, _ := c.Get("request_id")
				logger.Error("HTTP panic recovered", "request_id", requestID, "error", fmt.Sprint(recovered), "stack", string(debug.Stack()))
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			}
		}()
		c.Next()
	}
}

func newRequestID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err == nil {
		bytes[6] = (bytes[6] & 0x0f) | 0x40
		bytes[8] = (bytes[8] & 0x3f) | 0x80
		encoded := hex.EncodeToString(bytes[:])
		return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32]
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
