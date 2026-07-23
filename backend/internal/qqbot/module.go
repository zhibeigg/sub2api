package qqbot

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewConfigManager,
	NewOneBotConfigManager,
	NewReliableQueue,
	NewOneBotQueue,
	NewChannelCheckSigner,
	NewChannelStatusRenderer,
	NewChannelCheckService,
	NewRuntime,
	NewOneBotRuntime,
)
