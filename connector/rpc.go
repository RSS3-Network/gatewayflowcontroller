package connector

import (
	"context"

	"github.com/RSS3-Network/gatewayflowcontroller/types"
	"github.com/rss3-network/gateway-common/accesslog"
	"github.com/rss3-network/gateway-common/control"
)

type Connector struct {
	accesslogClient *accesslog.ProducerClient
	controlClient   *control.StateClientReader
}

func NewRPC(accesslogClient *accesslog.ProducerClient, controlClient *control.StateClientReader) *Connector {
	return &Connector{
		accesslogClient: accesslogClient,
		controlClient:   controlClient,
	}
}

func (c *Connector) AccesslogProduceLog(args *types.AccesslogProduceLogArgs, _ *struct{}) error {
	return c.accesslogClient.ProduceLog(&accesslog.Log{
		KeyID:     args.KeyID,
		Path:      args.Path,
		Status:    args.Status,
		Timestamp: args.Timestamp,
	})
}

func (c *Connector) ControlCheckKey(args *types.ControlCheckKeyArgs, reply *types.ControlCheckKeyReply) error {
	ctx := context.Background()
	account, keyID, err := c.controlClient.CheckKey(ctx, args.Key)
	reply.Account = account
	reply.KeyID = keyID

	return err
}

func (c *Connector) ControlCheckAccountPaused(args *types.CheckAccountPausedArgs, reply *types.CheckAccountPausedReply) error {
	ctx := context.Background()
	isPaused, err := c.controlClient.CheckAccountPaused(ctx, args.Account)
	reply.IsPaused = isPaused

	return err
}
