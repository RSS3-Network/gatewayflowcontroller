package GatewayFlowController_test

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"sync"
	"testing"
	"time"

	flowcontroller "github.com/RSS3-Network/GatewayFlowController"
	"github.com/RSS3-Network/GatewayFlowController/connector"
	"github.com/rss3-network/gateway-common/accesslog"
	"github.com/rss3-network/gateway-common/control"
)

const (
	MaxRequests = 1024 // Actually not so many... but just assign more to prevent stuck caused by overflow
)

//func TestClearMQPendings(t *testing.T) {
//
//	accesslogConsumer, _ := accesslog.NewConsumer([]string{"localhost:19092"}, "gateway.log.access", "gfc-test-clear")
//
//	_ = accesslogConsumer.Start(func(accessLog *accesslog.Log) {
//		t.Log(accessLog)
//	})
//
//	time.Sleep(10 * time.Second)
//
//	accesslogConsumer.Stop()
//}

func prepareRPC(t *testing.T) (string, func()) {
	// Prepare config
	KafkaBrokers := []string{"localhost:19092"}
	KafkaTopic := "gateway.log.access.test-full"
	EtcdEndpoints := []string{"localhost:2379"}

	// Create accesslog producer
	accesslogProducer, err := accesslog.NewProducer(KafkaBrokers, KafkaTopic)

	if err != nil {
		t.Fatal(fmt.Errorf("create accesslog producer: %w", err))
	}

	// Create control reader
	controlReader, err := control.NewReader(EtcdEndpoints)

	if err != nil {
		t.Fatal(fmt.Errorf("create control reader: %w", err))
	}

	// Prepare RPC listener
	listener, err := net.Listen("tcp", "127.0.0.1:")
	if err != nil {
		t.Fatal(fmt.Errorf("failed to start RPC listen: %w", err))
	}

	// Prepare Connector
	connectorRPC := connector.NewRPC(accesslogProducer, controlReader)

	if err = rpc.Register(connectorRPC); err != nil {
		log.Fatalf("failed to register rpc； %v", err)
	}

	go rpc.Accept(listener)

	return listener.Addr().String(), func() {
		//accesslogProducer.Stop()
		//controlReader.Stop()
		//_ = listener.Close()
	}
}

func TestFull(t *testing.T) {
	t.Parallel()

	// Prepare plugin
	cfg := flowcontroller.CreateConfig()

	cfg.Period = 30 * time.Second
	cfg.Average = 5
	cfg.Burst = 10

	// Prepare config
	KafkaBrokers := []string{"localhost:19092"}
	KafkaTopic := "gateway.log.access.test-full"
	EtcdEndpoints := []string{"localhost:2379"}

	// Prepare plugin handler
	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})

	rpcEndpoint, rpcStop := prepareRPC(t)
	defer rpcStop()

	cfg.ConnectorRPC = rpcEndpoint

	handler, err := flowcontroller.New(ctx, next, cfg, "gateway-flowcontroller-full")
	if err != nil {
		t.Fatal(fmt.Errorf("initialize plugin: %w", err))
	}

	// Create accesslog consumer
	accesslogConsumer, err := accesslog.NewConsumer(KafkaBrokers, KafkaTopic, "gfc-test-full")

	if err != nil {
		t.Fatal(fmt.Errorf("create accesslog consumer: %w", err))
	}

	defer accesslogConsumer.Stop()

	// Create control writer
	controlWriter, err := control.NewWriter(EtcdEndpoints)

	if err != nil {
		t.Fatal(fmt.Errorf("create control writer: %w", err))
	}

	defer controlWriter.Stop()

	// Prepare test case storage space
	receiveLogChan := make(chan accesslog.Log, MaxRequests)

	var wg sync.WaitGroup

	// Start consuming
	if err = accesslogConsumer.Start(func(accessLog *accesslog.Log) {
		//t.Logf("access log consume: %v", *accessLog)
		receiveLogChan <- *accessLog
		wg.Done()
	}); err != nil {
		t.Fatal(fmt.Errorf("start consuming: %w", err))
	}

	// Everything is ready, let's start requesting
	requestCountPerRound := cfg.Burst * 2 // Ensure exceeds burst stage
	demoKeyID := "83298679882397449001"
	demoKey := "84b01bc1-4dad-4694-99ce-514c37b88f9a-001"
	demoAccount := "0xD3E8ce4841ed658Ec8dcb99B7a74beFC377253EA-001"
	requestMethod := http.MethodGet
	requestURL := "http://localhost/gateway-flowcontroller"
	requestPath := "/gateway-flowcontroller"

	fullPhase1(ctx, t, cfg, handler, controlWriter, &wg, &receiveLogChan, requestCountPerRound, requestMethod, requestURL, requestPath, demoAccount, demoKeyID, demoKey)

	fullPhase2(ctx, t, cfg, handler, controlWriter, &wg, &receiveLogChan, requestCountPerRound, requestMethod, requestURL, requestPath, demoAccount, demoKeyID, demoKey)

	fullPhase3(ctx, t, cfg, handler, controlWriter, &wg, &receiveLogChan, requestCountPerRound, requestMethod, requestURL, requestPath, demoAccount, demoKeyID, demoKey)

	fullPhase4(ctx, t, cfg, handler, controlWriter, &wg, &receiveLogChan, requestCountPerRound, requestMethod, requestURL, requestPath, demoAccount, demoKeyID, demoKey)

	fullPhase5(ctx, t, cfg, handler, controlWriter, &wg, &receiveLogChan, requestCountPerRound, requestMethod, requestURL, requestPath, demoAccount, demoKeyID, demoKey)

	t.Log("full environment test finish")
}

func fullPhase1(ctx context.Context, t *testing.T, cfg *flowcontroller.Config, handler http.Handler, controlWriter *control.StateClientWriter, wg *sync.WaitGroup, receiveLogChan *chan accesslog.Log, requestCountPerRound int64, requestMethod string, requestURL string, requestPath string, demoAccount string, demoKeyID string, demoKey string) {
	// Phase 1: normal request with key
	var (
		err error
	)

	// Everything should be fine
	if err = controlWriter.CreateKey(ctx, demoAccount, demoKeyID, demoKey); err != nil {
		t.Fatal(fmt.Errorf("phase 1 create key: %w", err))
	}

	// Make requests
	for i := int64(0); i < requestCountPerRound; i++ {
		req, err := http.NewRequestWithContext(ctx, requestMethod, requestURL, nil)
		if err != nil {
			t.Fatal(fmt.Errorf("prepare request: %w", err))
		}

		req.Header.Set(cfg.KeyHeader, demoKey)
		req.Header.Set("X-Forwarded-For", "127.0.1.1, 192.168.0.1, 172.16.35.4")
		req.RemoteAddr = "127.0.1.1:12553"

		recorder := httptest.NewRecorder()

		// Make request
		wg.Add(1)
		handler.ServeHTTP(recorder, req)

		res := recorder.Result()

		if res.StatusCode != http.StatusOK {
			t.Error(fmt.Errorf("phase 1: unexpected status (%d) at response (%d)", res.StatusCode, i))
		}

		_ = res.Body.Close()
	}

	// Wait for done
	wg.Wait()

	// Validate requests
	for i := int64(0); i < requestCountPerRound; i++ {
		l := <-*receiveLogChan

		if l.KeyID == nil || *l.KeyID != demoKeyID {
			t.Error(fmt.Errorf("phase 1: unexpected key (%v) at received log (%d)", l.KeyID, i))
		}

		if l.Path != requestPath {
			t.Error(fmt.Errorf("phase 1: unexpedted path (%s) at received log (%d)", l.Path, i))
		}

		if l.Status != http.StatusOK {
			t.Error(fmt.Errorf("phase 1: unexpected status (%d) at received log (%d)", l.Status, i))
		}
	}
}

func fullPhase2(ctx context.Context, t *testing.T, cfg *flowcontroller.Config, handler http.Handler, controlWriter *control.StateClientWriter, wg *sync.WaitGroup, receiveLogChan *chan accesslog.Log, requestCountPerRound int64, requestMethod string, requestURL string, requestPath string, demoAccount string, demoKeyID string, demoKey string) {
	// Phase 2: paused account
	var (
		err error
	)

	// Should be rate limited with http 402 (Payment required), and with valid key record
	if err = controlWriter.PauseAccount(ctx, demoAccount); err != nil {
		t.Fatal(fmt.Errorf("phase 2 pause account: %w", err))
	}

	// Make requests
	for i := int64(0); i < requestCountPerRound; i++ {
		req, err := http.NewRequestWithContext(ctx, requestMethod, requestURL, nil)
		if err != nil {
			t.Fatal(fmt.Errorf("prepare request: %w", err))
		}

		req.Header.Set(cfg.KeyHeader, demoKey)
		req.Header.Set("X-Forwarded-For", "127.0.1.2, 192.168.0.1, 172.16.35.4")
		req.RemoteAddr = "127.0.1.2:12553"

		recorder := httptest.NewRecorder()

		// Make request
		wg.Add(1)
		handler.ServeHTTP(recorder, req)

		res := recorder.Result()

		if i >= cfg.Burst && res.StatusCode != http.StatusPaymentRequired {
			t.Error(fmt.Errorf("phase 2: unexpected status (%d) at response (%d)", res.StatusCode, i))
		}

		_ = res.Body.Close()
	}

	// Wait for done
	wg.Wait()

	// Validate requests
	for i := int64(0); i < requestCountPerRound; i++ {
		l := <-*receiveLogChan

		if l.KeyID == nil || *l.KeyID != demoKeyID {
			t.Error(fmt.Errorf("phase 2: unexpected key (%v) at received log (%d)", l.KeyID, i))
		}

		if l.Path != requestPath {
			t.Error(fmt.Errorf("phase 2: unexpedted path (%s) at received log (%d)", l.Path, i))
		}

		if i >= cfg.Burst && l.Status != http.StatusPaymentRequired {
			t.Error(fmt.Errorf("phase 2: unexpected status (%d) at received log (%d)", l.Status, i))
		}
	}
}

func fullPhase3(ctx context.Context, t *testing.T, cfg *flowcontroller.Config, handler http.Handler, _ *control.StateClientWriter, wg *sync.WaitGroup, receiveLogChan *chan accesslog.Log, requestCountPerRound int64, requestMethod string, requestURL string, requestPath string, _ string, _ string, _ string) {
	// Phase 3: without key
	// Should be rate limited
	// Make requests
	for i := int64(0); i < requestCountPerRound; i++ {
		req, err := http.NewRequestWithContext(ctx, requestMethod, requestURL, nil)
		if err != nil {
			t.Fatal(fmt.Errorf("prepare request: %w", err))
		}

		req.Header.Set("X-Forwarded-For", "127.0.1.3, 192.168.0.1, 172.16.35.4")
		req.RemoteAddr = "127.0.1.3:12553"

		recorder := httptest.NewRecorder()

		// Make request
		wg.Add(1)
		handler.ServeHTTP(recorder, req)

		res := recorder.Result()

		if i >= cfg.Burst && res.StatusCode != http.StatusTooManyRequests {
			t.Error(fmt.Errorf("phase 3: unexpected status (%d) at response (%d)", res.StatusCode, i))
		}

		_ = res.Body.Close()
	}

	// Wait for done
	wg.Wait()

	// Validate requests
	for i := int64(0); i < requestCountPerRound; i++ {
		l := <-*receiveLogChan

		if l.KeyID != nil {
			t.Error(fmt.Errorf("phase 3: unexpected key (%v) at received log (%d)", l.KeyID, i))
		}

		if l.Path != requestPath {
			t.Error(fmt.Errorf("phase 3: unexpedted path (%s) at received log (%d)", l.Path, i))
		}

		if i >= cfg.Burst && l.Status != http.StatusTooManyRequests {
			t.Error(fmt.Errorf("phase 3: unexpected status (%d) at received log (%d)", l.Status, i))
		}
	}
}

func fullPhase4(ctx context.Context, t *testing.T, cfg *flowcontroller.Config, handler http.Handler, controlWriter *control.StateClientWriter, wg *sync.WaitGroup, receiveLogChan *chan accesslog.Log, requestCountPerRound int64, requestMethod string, requestURL string, requestPath string, demoAccount string, demoKeyID string, demoKey string) {
	// Phase 4: resumed account
	var (
		err error
	)

	// Should be all fine
	if err = controlWriter.ResumeAccount(ctx, demoAccount); err != nil {
		t.Fatal(fmt.Errorf("phase 4 resume account: %w", err))
	}

	// Make requests
	for i := int64(0); i < requestCountPerRound; i++ {
		req, err := http.NewRequestWithContext(ctx, requestMethod, requestURL, nil)
		if err != nil {
			t.Fatal(fmt.Errorf("prepare request: %w", err))
		}

		req.Header.Set(cfg.KeyHeader, demoKey)
		req.Header.Set("X-Forwarded-For", "127.0.1.4, 192.168.0.1, 172.16.35.4")
		req.RemoteAddr = "127.0.1.4:12553"

		recorder := httptest.NewRecorder()

		// Make request
		wg.Add(1)
		handler.ServeHTTP(recorder, req)

		res := recorder.Result()

		if res.StatusCode != http.StatusOK {
			t.Error(fmt.Errorf("phase 4: unexpected status (%d) at response (%d)", res.StatusCode, i))
		}

		_ = res.Body.Close()
	}

	// Wait for done
	wg.Wait()

	// Validate requests
	for i := int64(0); i < requestCountPerRound; i++ {
		l := <-*receiveLogChan

		if l.KeyID == nil || *l.KeyID != demoKeyID {
			t.Error(fmt.Errorf("phase 4: unexpected key (%v) at received log (%d)", l.KeyID, i))
		}

		if l.Path != requestPath {
			t.Error(fmt.Errorf("phase 4: unexpedted path (%s) at received log (%d)", l.Path, i))
		}

		if l.Status != http.StatusOK {
			t.Error(fmt.Errorf("phase 4: unexpected status (%d) at received log (%d)", l.Status, i))
		}
	}
}

func fullPhase5(ctx context.Context, t *testing.T, cfg *flowcontroller.Config, handler http.Handler, controlWriter *control.StateClientWriter, wg *sync.WaitGroup, receiveLogChan *chan accesslog.Log, requestCountPerRound int64, requestMethod string, requestURL string, requestPath string, _ string, _ string, demoKey string) {
	// Phase 5: with invalid key
	var (
		err error
	)

	// Should be rate limited with http 429

	// Make requests
	for i := int64(0); i < requestCountPerRound; i++ {
		req, err := http.NewRequestWithContext(ctx, requestMethod, requestURL, nil)
		if err != nil {
			t.Fatal(fmt.Errorf("prepare request: %w", err))
		}

		req.Header.Set(cfg.KeyHeader, "some-meaningless-bytes")
		req.Header.Set("X-Forwarded-For", "127.0.1.5, 192.168.0.1, 172.16.35.4")
		req.RemoteAddr = "127.0.1.5:12553"

		recorder := httptest.NewRecorder()

		// Make request
		wg.Add(1)
		handler.ServeHTTP(recorder, req)

		res := recorder.Result()

		if i >= cfg.Burst && res.StatusCode != http.StatusTooManyRequests {
			t.Error(fmt.Errorf("phase 5: unexpected status (%d) at response (%d)", res.StatusCode, i))
		}

		_ = res.Body.Close()
	}

	// Wait for done
	wg.Wait()

	// Validate requests
	for i := int64(0); i < requestCountPerRound; i++ {
		l := <-*receiveLogChan

		if l.KeyID != nil {
			t.Error(fmt.Errorf("phase 5: unexpected key (%v) at received log (%d)", l.KeyID, i))
		}

		if l.Path != requestPath {
			t.Error(fmt.Errorf("phase 5: unexpedted path (%s) at received log (%d)", l.Path, i))
		}

		if i >= cfg.Burst && l.Status != http.StatusTooManyRequests {
			t.Error(fmt.Errorf("phase 5: unexpected status (%d) at received log (%d)", l.Status, i))
		}
	}

	// Clean up
	if err = controlWriter.DeleteKey(ctx, demoKey); err != nil {
		t.Error(fmt.Errorf("clean up delete key: %w", err))
	}
}
