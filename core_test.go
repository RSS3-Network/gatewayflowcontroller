package GatewayFlowController_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	flowcontroller "github.com/RSS3-Network/GatewayFlowController"
)

func TestCore(t *testing.T) {
	t.Parallel()

	// Prepare plugin
	cfg := flowcontroller.CreateConfig()

	ctx := context.Background()
	nextHandlerCalled := false
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) { nextHandlerCalled = true })

	handler, err := flowcontroller.New(ctx, next, cfg, "gateway-flowcontroller-core")
	if err != nil {
		t.Fatal(fmt.Errorf("initialize plugin: %w", err))
	}

	// Everything is ready, let's start requesting
	requestCountPerRound := cfg.Burst * 2 // Ensure exceeds burst stage
	requestMethod := http.MethodGet
	requestURL := "http://localhost/gateway-flowcontroller"

	corePhase1(ctx, t, cfg, handler, &nextHandlerCalled, requestCountPerRound, requestMethod, requestURL)

	corePhase2(ctx, t, cfg, handler, &nextHandlerCalled, requestCountPerRound, requestMethod, requestURL)

	t.Log("rate limiter only test finish")
}

func corePhase1(ctx context.Context, t *testing.T, cfg *flowcontroller.Config, handler http.Handler, nextHandlerCalled *bool, requestCountPerRound int64, requestMethod string, requestURL string) {
	// Phase 1: without key
	for i := int64(0); i < requestCountPerRound; i++ {
		req, err := http.NewRequestWithContext(ctx, requestMethod, requestURL, nil)
		if err != nil {
			t.Fatal(fmt.Errorf("prepare request: %w", err))
		}

		req.Header.Set("X-Forwarded-For", "127.0.2.1, 192.168.0.1, 172.16.35.4")
		req.RemoteAddr = "127.0.2.1:12553"

		recorder := httptest.NewRecorder()
		*nextHandlerCalled = false

		// Make request
		handler.ServeHTTP(recorder, req)

		res := recorder.Result()

		//t.Log(fmt.Sprintf("request %d, status %d, Retry-After: %s, X-Retry-In: %s", i, res.StatusCode, res.Header.Get("Retry-After"), res.Header.Get("X-Retry-In")))

		if i < cfg.Burst {
			if res.StatusCode != http.StatusOK {
				t.Error(fmt.Errorf("phase 1: unexpected status (%d) at response (%d)", res.StatusCode, i))
			}

			if !*nextHandlerCalled {
				t.Error(fmt.Errorf("phase 1: next handler not called at response (%d)", i))
			}
		} else {
			if res.StatusCode != http.StatusTooManyRequests {
				t.Error(fmt.Errorf("phase 1: unexpected status (%d) at response (%d)", res.StatusCode, i))
			}

			if *nextHandlerCalled {
				t.Error(fmt.Errorf("phase 1: next handler called at response (%d)", i))
			}
		}

		_ = res.Body.Close()
	}
}

func corePhase2(ctx context.Context, t *testing.T, cfg *flowcontroller.Config, handler http.Handler, nextHandlerCalled *bool, requestCountPerRound int64, requestMethod string, requestURL string) {
	// Phase 2: with invalid key
	for i := int64(0); i < requestCountPerRound; i++ {
		req, err := http.NewRequestWithContext(ctx, requestMethod, requestURL, nil)
		if err != nil {
			t.Fatal(fmt.Errorf("prepare request: %w", err))
		}

		req.Header.Set(cfg.KeyHeader, "some-meaningless-bytes")
		req.Header.Set("X-Forwarded-For", "127.0.2.2, 192.168.0.1, 172.16.35.4")
		req.RemoteAddr = "127.0.2.2:12553"

		recorder := httptest.NewRecorder()
		*nextHandlerCalled = false

		// Make request
		handler.ServeHTTP(recorder, req)

		res := recorder.Result()

		//t.Log(fmt.Sprintf("request %d, status %d, Retry-After: %s, X-Retry-In: %s", i, res.StatusCode, res.Header.Get("Retry-After"), res.Header.Get("X-Retry-In")))

		if i < cfg.Burst {
			if res.StatusCode != http.StatusOK {
				t.Error(fmt.Errorf("phase 2: unexpected status (%d) at response (%d)", res.StatusCode, i))
			}

			if !*nextHandlerCalled {
				t.Error(fmt.Errorf("phase 2: next handler not called at response (%d)", i))
			}
		} else {
			if res.StatusCode != http.StatusTooManyRequests {
				t.Error(fmt.Errorf("phase 2: unexpected status (%d) at response (%d)", res.StatusCode, i))
			}

			if *nextHandlerCalled {
				t.Error(fmt.Errorf("phase 2: next handler called at response (%d)", i))
			}
		}

		_ = res.Body.Close()
	}
}
