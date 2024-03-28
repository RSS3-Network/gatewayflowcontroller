package connector_test

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"sync"
	"testing"
	"time"

	"github.com/RSS3-Network/gatewayflowcontroller/connector"
	"github.com/RSS3-Network/gatewayflowcontroller/types"
	"github.com/rss3-network/gateway-common/accesslog"
	"github.com/rss3-network/gateway-common/control"
)

func prepareRPC(t *testing.T) (string, func()) {
	// Prepare config
	KafkaBrokers := []string{"localhost:19092"}
	KafkaTopic := "gateway.log.access.test-connector"
	EtcdEndpoints := []string{"localhost:2379"}

	// Create accesslog producer
	accesslogProducer, err := accesslog.NewProducer(KafkaBrokers, KafkaTopic)

	if err != nil {
		t.Fatal(fmt.Errorf("create accesslog producer: %w", err))
	}

	// Create control reader
	controlReader, err := control.NewReader(EtcdEndpoints, nil, nil)

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

	t.Log("rpc start")

	return listener.Addr().String(), func() {
		//accesslogProducer.Stop()
		//controlReader.Stop()
		//_ = listener.Close()
	}
}

func TestConnector(t *testing.T) {
	t.Parallel()

	rpcEndpoint, rpcStop := prepareRPC(t)
	defer rpcStop()

	rpcClient, err := rpc.Dial("tcp", rpcEndpoint)
	if err != nil {
		t.Fatal("failed to connect to rpc endpoint")
	}

	// Basically just the combination of Gateway-Common's tests, but now with rpc calls

	t.Run("accesslog", func(t *testing.T) {
		t.Parallel()
		testAccesslogRPC(t, rpcClient)
	})
	t.Run("control", func(t *testing.T) {
		t.Parallel()
		testControlRPC(t, rpcClient)
	})

	//twg.Wait()
	t.Log("test connector finish")
}

func testAccesslogRPC(t *testing.T, rpcClient *rpc.Client) {
	t.Log("accesslog test start")

	// Prepare configs
	brokers := []string{"localhost:19092"}
	topic := "gateway.log.access.test-connector"
	consumerGroup := "gateway-connector-test"

	// Create consumer
	t.Log("create consumer")

	consumer, err := accesslog.NewConsumer(brokers, topic, consumerGroup)

	if err != nil {
		t.Fatal(fmt.Errorf("create consumer: %w", err))
		return
	}

	defer consumer.Stop()

	// Prepare test case
	t.Log("prepare test case")

	demoLogs := []accesslog.Log{
		{
			KeyID:     nil, // No key
			Path:      "/foo",
			Status:    http.StatusOK,
			Timestamp: time.Unix(1710849419, 0),
		},
		{
			KeyID:     toPtr(t, "651654864321234"),
			Path:      "/bar",
			Status:    http.StatusTooManyRequests,
			Timestamp: time.Unix(1710849621, 0),
		},
		{
			KeyID:     nil, // No key
			Path:      "/baz",
			Status:    http.StatusInternalServerError,
			Timestamp: time.Unix(1710849652, 0),
		},
		{
			KeyID:     toPtr(t, "8645613456132156"),
			Path:      "/bar?alice=bob",
			Status:    http.StatusTooManyRequests,
			Timestamp: time.Unix(1710849711, 0),
		},
	}

	// Prepare message receive channel
	t.Log("prepare message receive channel")

	receiveLogChan := make(chan accesslog.Log, len(demoLogs)+1)

	var wg sync.WaitGroup

	// Start consuming
	t.Log("start consuming")

	if err = consumer.Start(func(accessLog *accesslog.Log) {
		t.Log("access log consume")
		receiveLogChan <- *accessLog
		wg.Done()
	}); err != nil {
		t.Error(err)
	}

	// Start producing
	t.Log("start producing")

	for _, l := range demoLogs {
		t.Log("access log produce")

		l := l

		if err = rpcClient.Call("Connector.AccesslogProduceLog", &types.AccesslogProduceLogArgs{
			KeyID:     l.KeyID,
			Path:      l.Path,
			Status:    l.Status,
			Timestamp: l.Timestamp,
		}, &struct{}{}); err != nil {
			t.Error(err)
		}

		wg.Add(1)
	}

	// Wait for all process finish
	t.Log("waiting all finish...")

	wg.Wait()

	// Close channel
	close(receiveLogChan)

	// Compare results
	counter := 0

	for receivedLog := range receiveLogChan {
		if !((receivedLog.KeyID == nil && demoLogs[counter].KeyID == nil) ||
			*receivedLog.KeyID == *demoLogs[counter].KeyID) {
			t.Error(fmt.Errorf("item %d key mismatch", counter))
		}

		if receivedLog.Path != demoLogs[counter].Path {
			t.Error(fmt.Errorf("item %d path mismatch", counter))
		}

		if receivedLog.Status != demoLogs[counter].Status {
			t.Error(fmt.Errorf("item %d status mismatch", counter))
		}

		if receivedLog.Timestamp.UnixNano() != demoLogs[counter].Timestamp.UnixNano() {
			t.Error(fmt.Errorf("item %d ts mismatch", counter))
		}

		counter++

		if counter > len(demoLogs) {
			break
		}
	}

	if counter != len(demoLogs) {
		t.Error("invalid logs length")
	}

	t.Log("accesslog test finish")
}

func testControlRPC(t *testing.T, rpcClient *rpc.Client) {
	// Prepare configs
	etcdEndpoints := []string{"localhost:2379"}

	// Create writer client
	writer, err := control.NewWriter(etcdEndpoints, nil, nil)

	if err != nil {
		t.Fatal(fmt.Errorf("create writer: %w", err))
		return
	}

	defer writer.Stop()

	// Prepare a key
	demoKey := "84b01bc1-4dad-4694-99ce-514c37b88f9a-002"
	demoKeyID := "83298679882397449002"
	// ...and an accounts
	demoAccount := "0xD3E8ce4841ed658Ec8dcb99B7a74beFC377253EA-002"
	// ...and context
	ctx := context.Background()

	// Test flow

	controlState0(ctx, t, rpcClient, writer, demoKey, demoKeyID, demoAccount)

	controlState1(ctx, t, rpcClient, writer, demoKey, demoKeyID, demoAccount)

	controlState2(ctx, t, rpcClient, writer, demoKey, demoKeyID, demoAccount)

	controlState3(ctx, t, rpcClient, writer, demoKey, demoKeyID, demoAccount)

	controlState4(ctx, t, rpcClient, writer, demoKey, demoKeyID, demoAccount)

	t.Log("control test finish")
}

func controlState0(_ context.Context, t *testing.T, rpcClient *rpc.Client, _ *control.StateClientWriter, demoKey string, _ string, demoAccount string) {
	//// State 0: nothing
	var checkKeyReply types.ControlCheckKeyReply
	if err := rpcClient.Call("Connector.ControlCheckKey", &types.ControlCheckKeyArgs{
		Key: demoKey,
	}, &checkKeyReply); err != nil {
		t.Error(fmt.Errorf("check key: %w", err))
	} else if checkKeyReply.Account != nil || checkKeyReply.KeyID != nil {
		t.Error("key should not exist")
	}

	var checkAccountPausedReply types.CheckAccountPausedReply
	if err := rpcClient.Call("Connector.ControlCheckAccountPaused", &types.CheckAccountPausedArgs{
		Account: demoAccount,
	}, &checkAccountPausedReply); err != nil {
		t.Error(fmt.Errorf("check account: %w", err))
	} else if checkAccountPausedReply.IsPaused {
		t.Error("account should not exist")
	}
}

func controlState1(ctx context.Context, t *testing.T, rpcClient *rpc.Client, writer *control.StateClientWriter, demoKey string, demoKeyID string, demoAccount string) {
	//// State 1: create key, no paused account
	if err := writer.CreateKey(ctx, demoAccount, demoKeyID, demoKey); err != nil {
		t.Error(fmt.Errorf("create key: %w", err))
	}

	var checkKeyReply types.ControlCheckKeyReply
	if err := rpcClient.Call("Connector.ControlCheckKey", &types.ControlCheckKeyArgs{
		Key: demoKey,
	}, &checkKeyReply); err != nil {
		t.Error(fmt.Errorf("check key: %w", err))
	} else if checkKeyReply.Account == nil || checkKeyReply.KeyID == nil {
		t.Error("key should exist")
	} else if *checkKeyReply.Account != demoAccount || *checkKeyReply.KeyID != demoKeyID {
		t.Error("wrong key info")
	}

	var checkAccountPausedReply types.CheckAccountPausedReply
	if err := rpcClient.Call("Connector.ControlCheckAccountPaused", &types.CheckAccountPausedArgs{
		Account: demoAccount,
	}, &checkAccountPausedReply); err != nil {
		t.Error(fmt.Errorf("check account: %w", err))
	} else if checkAccountPausedReply.IsPaused {
		t.Error("account should not exist")
	}
}

func controlState2(ctx context.Context, t *testing.T, rpcClient *rpc.Client, writer *control.StateClientWriter, demoKey string, demoKeyID string, demoAccount string) {
	// State 2: create key, pause account
	if err := writer.PauseAccount(ctx, demoAccount); err != nil {
		t.Error(fmt.Errorf("pause account: %w", err))
	}

	var checkKeyReply types.ControlCheckKeyReply

	if err := rpcClient.Call("Connector.ControlCheckKey", &types.ControlCheckKeyArgs{
		Key: demoKey,
	}, &checkKeyReply); err != nil {
		t.Error(fmt.Errorf("check key: %w", err))
	} else if checkKeyReply.Account == nil || checkKeyReply.KeyID == nil {
		t.Error("key should exist")
	} else if *checkKeyReply.Account != demoAccount || *checkKeyReply.KeyID != demoKeyID {
		t.Error("wrong key info")
	}

	var checkAccountPausedReply types.CheckAccountPausedReply
	if err := rpcClient.Call("Connector.ControlCheckAccountPaused", &types.CheckAccountPausedArgs{
		Account: demoAccount,
	}, &checkAccountPausedReply); err != nil {
		t.Error(fmt.Errorf("check account: %w", err))
	} else if !checkAccountPausedReply.IsPaused {
		t.Error("account should exist")
	}
}

func controlState3(ctx context.Context, t *testing.T, rpcClient *rpc.Client, writer *control.StateClientWriter, demoKey string, _ string, demoAccount string) {
	// State 3: no key, pause account
	if err := writer.DeleteKey(ctx, demoKey); err != nil {
		t.Error(fmt.Errorf("delete key: %w", err))
	}

	var checkKeyReply types.ControlCheckKeyReply
	if err := rpcClient.Call("Connector.ControlCheckKey", &types.ControlCheckKeyArgs{
		Key: demoKey,
	}, &checkKeyReply); err != nil {
		t.Error(fmt.Errorf("check key: %w", err))
	} else if checkKeyReply.Account != nil || checkKeyReply.KeyID != nil {
		t.Error("key should not exist")
	}

	var checkAccountPausedReply types.CheckAccountPausedReply
	if err := rpcClient.Call("Connector.ControlCheckAccountPaused", &types.CheckAccountPausedArgs{
		Account: demoAccount,
	}, &checkAccountPausedReply); err != nil {
		t.Error(fmt.Errorf("check account: %w", err))
	} else if !checkAccountPausedReply.IsPaused {
		t.Error("account should exist")
	}
}

func controlState4(ctx context.Context, t *testing.T, rpcClient *rpc.Client, writer *control.StateClientWriter, demoKey string, _ string, demoAccount string) {
	// State 4: nothing again
	if err := writer.ResumeAccount(ctx, demoAccount); err != nil {
		t.Error(fmt.Errorf("resume account: %w", err))
	}

	var checkKeyReply types.ControlCheckKeyReply
	if err := rpcClient.Call("Connector.ControlCheckKey", &types.ControlCheckKeyArgs{
		Key: demoKey,
	}, &checkKeyReply); err != nil {
		t.Error(fmt.Errorf("check key: %w", err))
	} else if checkKeyReply.Account != nil || checkKeyReply.KeyID != nil {
		t.Error("key should not exist")
	}

	var checkAccountPausedReply types.CheckAccountPausedReply
	if err := rpcClient.Call("Connector.ControlCheckAccountPaused", &types.CheckAccountPausedArgs{
		Account: demoAccount,
	}, &checkAccountPausedReply); err != nil {
		t.Error(fmt.Errorf("check account: %w", err))
	} else if checkAccountPausedReply.IsPaused {
		t.Error("account should not exist")
	}
}

func toPtr[T any](t *testing.T, v T) *T {
	t.Helper()

	return &v
}
