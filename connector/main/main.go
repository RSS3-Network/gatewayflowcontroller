package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"os"

	"github.com/RSS3-Network/GatewayFlowController/connector"
	"github.com/rss3-network/gateway-common/accesslog"
	"github.com/rss3-network/gateway-common/control"
	"gopkg.in/yaml.v3"
)

var (
	flagConfigFilePath string
)

func init() {
	flag.StringVar(&flagConfigFilePath, "c", "config.yml", "Path for configuration file")
}

func prepare() (*net.Listener, *accesslog.ProducerClient, *control.StateClientReader, error) {
	var (
		config          connector.Config
		accesslogClient *accesslog.ProducerClient
		controlClient   *control.StateClientReader
	)

	// Read config
	configBytes, err := os.ReadFile(flagConfigFilePath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse config
	err = yaml.Unmarshal(configBytes, &config)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to unmarshal config file: %w", err)
	}

	// Prepare listener
	listener, err := net.Listen("tcp", config.Listen)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to listen on (%s): %w", config.Listen, err)
	}

	// Initialize components
	if len(config.KafkaBrokers) > 0 {
		// Initialize accesslog
		if accesslogClient, err = accesslog.NewProducer(config.KafkaBrokers, config.KafkaTopic); err != nil {
			return nil, nil, nil, fmt.Errorf("init accesslog: %w", err)
		}
	} else {
		return nil, nil, nil, fmt.Errorf("missing kafka brokers")
	}

	if len(config.EtcdEndpoints) > 0 {
		// Initialize control
		if controlClient, err = control.NewReader(config.EtcdEndpoints); err != nil {
			return nil, nil, nil, fmt.Errorf("init controller: %w", err)
		}
	} else {
		return nil, nil, nil, fmt.Errorf("missing etcd endpoints")
	}

	return &listener, accesslogClient, controlClient, nil
}

func main() {
	flag.Parse()

	listener, accesslogClient, controlClient, err := prepare()
	if err != nil {
		log.Fatalf("failed to prepare； %v", err)
	}

	defer (*listener).Close()
	defer accesslogClient.Stop()
	defer controlClient.Stop()

	connectorRPC := connector.NewRPC(accesslogClient, controlClient)

	if err = rpc.Register(connectorRPC); err != nil {
		log.Fatalf("failed to register rpc； %v", err)
	}

	rpc.Accept(*listener)
}
