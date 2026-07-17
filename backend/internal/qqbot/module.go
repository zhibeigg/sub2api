package qqbot

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewConfigManager,
	NewReliableQueue,
	NewRuntime,
)
