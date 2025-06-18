package main

import (
	"github.com/extism/go-pdk"

	bus "github.com/autonomouskoi/core-tinygo"
	twitchchat "github.com/autonomouskoi/trackstar-twitchchat/go"
)

var (
	c *twitchchat.Chat
)

//go:export start
func Initialize() int32 {
	bus.LogDebug("starting up")

	var err error
	c, err = twitchchat.New()
	if err != nil {
		bus.LogError("loading config", "error", err.Error())
		return -1
	}

	return 0
}

//go:export recv
func Recv() int32 {
	msg := &bus.BusMessage{}
	if err := msg.UnmarshalVT(pdk.Input()); err != nil {
		bus.LogError("unmarshalling message", "error", err.Error())
		return 0
	}
	c.Handle(msg)
	return 0
}

func main() {}
