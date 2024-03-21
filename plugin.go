package GatewayFlowController

import (
	"context"
	"fmt"
	"net/http"
	"net/rpc"
	"os"
	"time"

	"github.com/RSS3-Network/GatewayFlowController/types"
)

// Config the plugin configuration.
type Config struct {
	// Request related
	KeyHeader string `json:"key_header,omitempty"`

	// Connector
	ConnectorRPC string `json:"connector_rpc,omitempty"`

	// Rate Limit
	MaxSources int           `json:"max_sources,omitempty"`
	Average    int64         `json:"average,omitempty"`
	Period     time.Duration `json:"period,omitempty"`
	Burst      int64         `json:"burst,omitempty"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		KeyHeader:    "X-API-Key", // Default header
		ConnectorRPC: "",          // Disable connector
		MaxSources:   65535,
		Average:      20,
		Period:       time.Minute,
		Burst:        60,
	}
}

type Core struct {
	next http.Handler
	name string

	rateLimiter *RateLimiter
}

// FlowController ： a FlowController plugin.
type FlowController struct {
	Core

	keyHeader string
	connector *rpc.Client
}

// New created a new plugin.
func New(_ context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	var (
		connector *rpc.Client
		err       error
	)

	// Whether to use full mode
	fullMode := true

	if config.ConnectorRPC != "" {
		connector, err = rpc.Dial("tcp", config.ConnectorRPC)
		if err != nil {
			return nil, fmt.Errorf("init connector: %w", err)
		}
	} else {
		_, _ = os.Stderr.WriteString("no connector rpc found, skip connector init\n")
		fullMode = false
	}

	// Initialize rate limiter
	rateLimiter, err := NewRateLimiter(config.MaxSources, config.Average, config.Period, config.Burst)
	if err != nil {
		return nil, fmt.Errorf("init rate limiter: %w", err)
	}

	core := Core{
		next: next,
		name: name,

		rateLimiter: rateLimiter,
	}

	if !fullMode {
		// Only enable core mode
		_, _ = os.Stderr.WriteString("fallback to core mode\n")
		return &core, nil
	}

	// Enable full mode
	return &FlowController{
		Core: core,

		keyHeader: config.KeyHeader,
		connector: connector,
	}, nil
}

// The Core processor
func (fc *Core) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if fc.rateLimiter.RateLimit(rw, req, nil) {
		fc.next.ServeHTTP(rw, req)
	}
}

// The Full processor
func (fc *FlowController) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// Prepare the access log
	l := types.AccesslogProduceLogArgs{
		Path:      req.URL.Path,
		Timestamp: time.Now(),
	}

	// To get response code, we have to proxy the response writer.
	rwp := NewResponseWriterProxy(rw)

	// Whether proceed to next middleware
	proceed := true

	if key := req.Header.Get(fc.keyHeader); key == "" {
		// No key provided, use common rate limit
		proceed = fc.rateLimiter.RateLimit(rwp, req, nil)
	} else {
		// Query key
		var checkKeyReply types.ControlCheckKeyReply

		if err := fc.connector.Call("Connector.ControlCheckKey", &types.ControlCheckKeyArgs{
			Key: key,
		}, &checkKeyReply); err != nil {
			// Key provided, but failed to check, apply rate limit
			_, _ = os.Stderr.WriteString(fmt.Sprintf("check key %s: %v\n", key, err))
			proceed = fc.rateLimiter.RateLimit(rwp, req, nil)
		} else if checkKeyReply.Account == nil {
			// Key check succeeded, but no such key
			proceed = fc.rateLimiter.RateLimit(rwp, req, nil)
		} else {
			// Has valid key, record it
			l.KeyID = checkKeyReply.KeyID

			var checkAccountPausedReply types.CheckAccountPausedReply

			// Check whether the account of this key has been paused
			if err := fc.connector.Call("Connector.ControlCheckAccountPaused", &types.CheckAccountPausedArgs{
				Account: *checkKeyReply.Account,
			}, &checkAccountPausedReply); err != nil {
				// Failed to check account, skip rate limit
				_, _ = os.Stderr.WriteString(fmt.Sprintf("check account %s: %v\n", *checkKeyReply.Account, err))
			} else if checkAccountPausedReply.IsPaused {
				// Account paused, apply rate limit by account
				proceed = fc.rateLimiter.RateLimit(rwp, req, checkKeyReply.Account)
			}
		}
	}

	// Should we proceed or abort ?
	if proceed {
		fc.next.ServeHTTP(rwp, req)
	}

	// Produce log
	l.Status = rwp.StatusCode()
	go func() {
		l := l
		if err := fc.connector.Call("Connector.AccesslogProduceLog", &l, nil); err != nil {
			_, _ = os.Stderr.WriteString(fmt.Sprintf("produce log %v: %v\n", l, err))
		}
	}()
}

// Stop will not be called, it's just a resource release reminder
func (fc *FlowController) Stop() {
	_ = fc.connector.Close()
}
