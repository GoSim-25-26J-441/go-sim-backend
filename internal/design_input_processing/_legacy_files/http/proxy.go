package http

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

// proxyResponse forwards an HTTP response from upstream to the client
func proxyResponse(c *gin.Context, resp *http.Response) {
	// Copy headers
	for k, v := range resp.Header {
		if len(v) > 0 {
			c.Header(k, v[0])
		}
	}
	
	// Set status and copy body
	c.Status(resp.StatusCode)
	_, _ = io.Copy(c.Writer, resp.Body)
}

// proxyResponseWithBody forwards an HTTP response with a modified body
func proxyResponseWithBody(c *gin.Context, resp *http.Response, body []byte) {
	// Copy headers
	for k, vs := range resp.Header {
		if len(vs) > 0 {
			c.Header(k, vs[0])
		}
	}
	
	// Set status and write body
	c.Status(resp.StatusCode)
	_, _ = c.Writer.Write(body)
}
