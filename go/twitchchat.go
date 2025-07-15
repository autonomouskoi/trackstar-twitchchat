package twitchchat

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/autonomouskoi/akcore"
	bus "github.com/autonomouskoi/core-tinygo"
	"github.com/autonomouskoi/core-tinygo/svc"
	trackstar "github.com/autonomouskoi/trackstar-tinygo"
	twitch "github.com/autonomouskoi/twitch-tinygo"
)

const (
	defaultTemplate = `{{ .track_update.track.artist }} - {{ .track_update.track.title }}`
)

var (
	topic_request = BusTopics_TRACKSTAR_TWITCH_CHAT_REQUEST.String()
	topic_command = BusTopics_TRACKSTAR_TWITCH_CHAT_COMMAND.String()

	topic_trackstar_event = trackstar.BusTopic_TRACKSTAR_EVENT.String()

	topic_chat_event = twitch.BusTopics_TWITCH_EVENTSUB_EVENT.String()

	cfgKVKey = []byte("config")
)

type Chat struct {
	cfg    *Config
	router bus.TopicRouter
}

func New() (*Chat, error) {
	for _, topic := range []string{topic_request, topic_command, topic_chat_event, topic_trackstar_event} {
		bus.LogDebug("subscribing", "topic", topic)
		if err := bus.Subscribe(topic); err != nil {
			return nil, fmt.Errorf("subscribing to %s", topic)
		}
		bus.LogDebug("subscribed", "topic", topic)
	}

	c := &Chat{
		cfg: &Config{},
	}
	if err := c.loadCfg(); err != nil && !errors.Is(err, akcore.ErrNotFound) {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	if c.cfg.SendAs == "" {
		resp, err := twitch.ListProfiles(&twitch.ListProfilesRequest{})
		if err != nil {
			return nil, fmt.Errorf("listing twitch profile: %w", err)
		}
		if len(resp.Names) == 0 {
			return nil, fmt.Errorf("no available profiles")
		}
		c.cfg.SendAs = resp.Names[0]
		if err := c.writeCfg(); err != nil {
			return nil, fmt.Errorf("writing config: %w", err)
		}
	}
	if c.cfg.SendTo == "" {
		c.cfg.SendAs = c.cfg.SendTo
		if err := c.writeCfg(); err != nil {
			return nil, fmt.Errorf("writing config: %w", err)
		}
	}

	if strings.TrimSpace(c.cfg.Template) == "" {
		c.cfg.Template = defaultTemplate
	}

	c.router = bus.TopicRouter{
		topic_request: bus.TypeRouter{
			int32(MessageTypeRequest_CONFIG_GET_REQ):     c.handleRequestGetConfig,
			int32(MessageTypeRequest_TRACK_ANNOUNCE_REQ): c.handleRequestTrackAnnounce,
		},
		topic_command: bus.TypeRouter{
			int32(MessageTypeCommand_CONFIG_SET_REQ): c.handleCommandSetConfig,
		},
		topic_chat_event: bus.TypeRouter{
			int32(twitch.MessageTypeEventSub_TYPE_CHANNEL_CHAT_MESSAGE): c.handleChatMessage,
		},
		topic_trackstar_event: bus.TypeRouter{
			int32(trackstar.MessageTypeEvent_TRACK_UPDATE): c.handleTrackUpdate,
		},
	}
	return c, nil
}

func (c *Chat) loadCfg() error {
	return bus.KVGetProto(cfgKVKey, c.cfg)
}

func (c *Chat) Handle(msg *bus.BusMessage) {
	c.router.Handle(msg)
}

func (c *Chat) handleRequestGetConfig(msg *bus.BusMessage) *bus.BusMessage {
	reply := bus.DefaultReply(msg)
	bus.MarshalMessage(reply, &ConfigGetResponse{Config: c.cfg})
	return reply
}

func (c *Chat) writeCfg() error {
	if err := bus.KVSetProto(cfgKVKey, c.cfg); err != nil {
		bus.LogError("saving config", "error", err.Error())
		return err
	}
	return nil
}

func (c *Chat) handleCommandSetConfig(msg *bus.BusMessage) *bus.BusMessage {
	reply := bus.DefaultReply(msg)
	req := ConfigSetRequest{}
	if reply.Error = bus.UnmarshalMessage(msg, &req); reply.Error != nil {
		return reply
	}

	c.cfg = req.GetConfig()
	if err := c.writeCfg(); err != nil {
		errStr := err.Error()
		bus.LogError("saving config", "error", errStr)
		reply.Error = &bus.Error{
			UserMessage: &errStr,
		}
		return reply
	}
	bus.MarshalMessage(reply, &ConfigSetResponse{Config: c.cfg})
	return reply
}

func (c *Chat) handleTrackUpdate(msg *bus.BusMessage) *bus.BusMessage {
	if !c.cfg.Announce {
		return nil
	}
	tu := &trackstar.TrackUpdate{}
	if err := bus.UnmarshalMessage(msg, tu); err != nil {
		return nil
	}
	c.sendTrackUpdate(tu)
	return nil
}

func (c *Chat) sendTrackUpdate(tu *trackstar.TrackUpdate) {
	b, err := tu.MarshalJSON()
	if err != nil {
		bus.LogError("marshalling TrackUpdate to JSON", "error", err.Error())
		return
	}
	json := bytes.Buffer{}
	json.WriteString("{")
	json.WriteString(`"track_update": `)
	json.Write(b)
	json.WriteString("}")
	output, renderErr := svc.RenderTemplate(c.cfg.GetTemplate(), json.Bytes())
	if renderErr != nil {
		bus.LogError("rendering template", "error", renderErr)
		return
	}

	msg := &bus.BusMessage{
		Topic: twitch.BusTopics_TWITCH_CHAT_REQUEST.String(),
		Type:  int32(twitch.MessageTypeTwitchChatRequest_TWITCH_CHAT_REQUEST_TYPE_SEND_REQ),
	}
	bus.MarshalMessage(msg, &twitch.TwitchChatRequestSendRequest{
		Text:    output,
		Profile: c.cfg.SendAs,
		Channel: c.cfg.SendTo,
	})
	if msg.Error != nil {
		return
	}
	bus.Send(msg)
}

func (c *Chat) handleChatMessage(msg *bus.BusMessage) *bus.BusMessage {
	ccm := &twitch.EventChannelChatMessage{}
	if err := bus.UnmarshalMessage(msg, ccm); err != nil {
		return nil
	}
	text := strings.ToLower(ccm.GetMessage().Text)
	if !strings.HasPrefix(text, "!id") {
		return nil
	}

	req := &trackstar.GetTrackRequest{}
	for _, arg := range strings.Split(text, " ")[1:] {
		arg = strings.TrimSpace(arg)
		if arg != "" {
			text = arg
			break
		}
	}
	if text != "" {
		duration, err := time.ParseDuration(text)
		if err == nil {
			req.DeltaSeconds = uint32(duration / time.Second)
		}
	}

	reqMsg := &bus.BusMessage{
		Topic: trackstar.BusTopic_TRACKSTAR_REQUEST.String(),
		Type:  int32(trackstar.MessageTypeRequest_GET_TRACK_REQ),
	}
	if bus.MarshalMessage(reqMsg, req); reqMsg.Error != nil {
		return nil
	}
	reply, err := bus.WaitForReply(reqMsg, 5000)
	if err != nil {
		return nil
	}
	gtr := &trackstar.GetTrackResponse{}
	if err := bus.UnmarshalMessage(reply, gtr); err != nil {
		return nil
	}
	c.sendTrackUpdate(gtr.TrackUpdate)
	return nil
}

func (c *Chat) handleRequestTrackAnnounce(msg *bus.BusMessage) *bus.BusMessage {
	reply := bus.DefaultReply(msg)
	tsReq := &bus.BusMessage{
		Topic: trackstar.BusTopic_TRACKSTAR_REQUEST.String(),
		Type:  int32(trackstar.MessageTypeRequest_GET_TRACK_REQ),
	}
	if bus.MarshalMessage(tsReq, &trackstar.GetTrackRequest{}); tsReq.Error != nil {
		return nil
	}
	tsReply, err := bus.WaitForReply(tsReq, 1000)
	if err != nil {
		bus.LogError("waiting for reply",
			"operation", "requesting tracK",
			"error", err.Error(),
		)
		return nil
	}
	if tsReply.Error != nil {
		reply.Error = tsReply.Error
		return reply
	}
	gtr := &trackstar.GetTrackResponse{}
	if reply.Error = bus.UnmarshalMessage(tsReply, gtr); reply.Error != nil {
		return reply
	}
	c.sendTrackUpdate(gtr.GetTrackUpdate())
	return reply
}
