package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestServeHTTPAuthorizationRequired(t *testing.T) {
	assert := assert.New(t)
	plugin := Plugin{}
	plugin.router = plugin.initRouter()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/reports", nil)

	plugin.ServeHTTP(nil, w, r)

	result := w.Result()
	assert.NotNil(result)
	defer func() { _ = result.Body.Close() }()
	assert.Equal(http.StatusUnauthorized, result.StatusCode)
}

func TestRemainingLabel(t *testing.T) {
	assert := assert.New(t)
	assert.Equal("5 minutes", remainingLabel(5*60*1000))
	assert.Equal("1 minutes", remainingLabel(30*1000))
	assert.Equal("1 hours", remainingLabel(60*60*1000))
	assert.Equal("24 hours", remainingLabel(24*60*60*1000))
	assert.Equal("7 days", remainingLabel(7*24*60*60*1000))
}

func TestTruncate(t *testing.T) {
	assert := assert.New(t)
	assert.Equal("short", truncate("short", 10))
	assert.Equal("aaaa…", truncate("aaaaaaaaaa", 5))
}
