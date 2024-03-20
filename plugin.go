package flowcontroller

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/rss3-network/gateway-common/accesslog"
	"github.com/rss3-network/gateway-common/control"
)

// Config the plugin configuration.
type Config struct {
	// Request related
	KeyHeader string

	// Access Log Report
	KafkaBrokers []string
	KafkaTopic   string

	// State management
	EtcdEndpoints []string

	// Rate Limit
	MaxSources int
	Average    int64
	Period     time.Duration
	Burst      int64
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		KeyHeader:     "X-API-Key", // Default header
		KafkaBrokers:  []string{"localhost:19092"},
		KafkaTopic:    "gateway.log.access",
		EtcdEndpoints: []string{"localhost:2379"},
		MaxSources:    65535,
		Average:       20,
		Period:        time.Minute,
		Burst:         60,
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

	keyHeader       string
	accesslogClient *accesslog.ProducerClient
	controlClient   *control.StateClientReader
}

// New created a new plugin.
func New(_ context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	var (
		accesslogClient *accesslog.ProducerClient  //= nil
		controlClient   *control.StateClientReader //= nil
		err             error
	)

	// Whether to use full mode
	fullMode := true

	if len(config.KafkaBrokers) > 0 {
		// Initialize accesslog
		accesslogClient, err = accesslog.NewProducer(config.KafkaBrokers, config.KafkaTopic)
		if err != nil {
			return nil, fmt.Errorf("init accesslog: %w", err)
		}
	} else {
		_, _ = os.Stderr.WriteString("no kafka brokers found, skip accesslog init\n")
		fullMode = false
	}

	if len(config.EtcdEndpoints) > 0 {
		// Initialize control
		controlClient, err = control.NewReader(config.EtcdEndpoints)
		if err != nil {
			return nil, fmt.Errorf("init controller: %w", err)
		}
	} else {
		_, _ = os.Stderr.WriteString("no etcd endpoints found, skip controller init\n")
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

		keyHeader:       config.KeyHeader,
		accesslogClient: accesslogClient,
		controlClient:   controlClient,
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
	// Prepare context
	ctx := req.Context()

	// Prepare the access log
	l := accesslog.Log{
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
	} else if account, keyID, err := fc.controlClient.CheckKey(ctx, key); err != nil {
		// Key provided, but failed to check, apply rate limit
		_, _ = os.Stderr.WriteString(fmt.Sprintf("check key %s: %v\n", key, err))
		proceed = fc.rateLimiter.RateLimit(rwp, req, nil)
	} else if account == nil {
		// Key check succeeded, but no such key
		proceed = fc.rateLimiter.RateLimit(rwp, req, nil)
	} else {
		// Has valid key, record it
		l.KeyID = keyID

		// Check whether the account of this key has been paused
		if paused, err := fc.controlClient.CheckAccountPaused(ctx, *account); err != nil {
			// Failed to check account, skip rate limit
			_, _ = os.Stderr.WriteString(fmt.Sprintf("check account %s: %v\n", *account, err))
		} else if paused {
			// Account paused, apply rate limit by account
			proceed = fc.rateLimiter.RateLimit(rwp, req, account)
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
		if err := fc.accesslogClient.ProduceLog(&l); err != nil {
			_, _ = os.Stderr.WriteString(fmt.Sprintf("produce log %v: %v\n", l, err))
		}
	}()
}

// Stop will not be called, it's just a resource release reminder
func (fc *FlowController) Stop() {
	fc.accesslogClient.Stop()
	fc.controlClient.Stop()
}
