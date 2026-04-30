package middleware

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

const requestBodyTooLargeKey = "shepherd.request_body_too_large"

type maxBytesReadCloser struct {
	body io.ReadCloser
	c    *gin.Context
}

func (r *maxBytesReadCloser) Read(p []byte) (int, error) {
	n, err := r.body.Read(p)
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) && r.c != nil {
		r.c.Set(requestBodyTooLargeKey, true)
	}
	return n, err
}

func (r *maxBytesReadCloser) Close() error {
	return r.body.Close()
}

func requestBodyTooLarge(c *gin.Context) bool {
	if c == nil {
		return false
	}
	value, ok := c.Get(requestBodyTooLargeKey)
	if !ok {
		return false
	}
	tooLarge, _ := value.(bool)
	return tooLarge
}

// MaxRequestBodyBytes limits inbound request bodies when a positive limit is configured.
func MaxRequestBodyBytes(limit int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if limit <= 0 || c.Request == nil || c.Request.Body == nil {
			c.Next()
			return
		}
		if c.Request.ContentLength > limit {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
				"code":    "REQUEST_TOO_LARGE",
				"message": "request body is too large",
			})
			return
		}
		c.Request.Body = &maxBytesReadCloser{
			body: http.MaxBytesReader(c.Writer, c.Request.Body, limit),
			c:    c,
		}
		c.Next()
	}
}
