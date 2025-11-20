package server_test

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	rpc "github.com/shortlink-org/go-sdk/grpc"
	"github.com/shortlink-org/go-sdk/logger"
	"github.com/shortlink-org/go-sdk/raft/server"
	v1 "github.com/shortlink-org/go-sdk/raft/v1"
)

func Test_Raft(t *testing.T) {
	ctx := t.Context()

	// Init logger
	conf := logger.Configuration{
		Level: logger.INFO_LEVEL,
	}
	log, err := logger.New(conf)
	require.NoError(t, err, "Error init a logger")

	// Step 1. Create 3 nodes ===================================================
	peers := []string{"127.0.0.1:50051", "127.0.0.1:50052", "127.0.0.1:50053"}

	// node 1 -----------------------------------------------------
	//nolint:mnd,revive // It's okay to have magic numbers here
	viper.Set("GRPC_SERVER_PORT", 50051)
	serverRPC1, err := rpc.InitServer(ctx, log, nil, nil)
	require.NoError(t, err)

	node1, err := server.New(ctx, serverRPC1, peers, server.WithLogger(log))
	require.NoError(t, err)

	// node 2 -----------------------------------------------------
	//nolint:mnd,revive // It's okay to have magic numbers here
	viper.Set("GRPC_SERVER_PORT", 50052)
	serverRPC2, err := rpc.InitServer(ctx, log, nil, nil)
	require.NoError(t, err)

	node2, err := server.New(ctx, serverRPC2, peers, server.WithLogger(log))
	require.NoError(t, err)

	// node 3 -----------------------------------------------------
	//nolint:mnd,revive // It's okay to have magic numbers here
	viper.Set("GRPC_SERVER_PORT", 50053)
	serverRPC3, err := rpc.InitServer(ctx, log, nil, nil)
	require.NoError(t, err)

	node3, err := server.New(ctx, serverRPC3, peers, server.WithLogger(log))
	require.NoError(t, err)

	// Check the status of nodes. All nodes should be in follower status
	require.Equal(t, v1.RaftStatus_RAFT_STATUS_FOLLOWER, node1.GetStatus())
	require.Equal(t, v1.RaftStatus_RAFT_STATUS_FOLLOWER, node2.GetStatus())
	require.Equal(t, v1.RaftStatus_RAFT_STATUS_FOLLOWER, node3.GetStatus())
}
