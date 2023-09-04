package mpkg

// ATMessageEventHandler at 机器人消息事件 handler
type ATMessageEventHandler func(event *WSPayload, data *Message) error
type eventParseFunc func(event *WSPayload, message []byte) error

// 管理所有支持的 handler 类型
var DefaultHandlers struct {
	ATMessage ATMessageEventHandler
}

// 注册事件回调，并返回 intent 用于 websocket 的鉴权
func RegisterHandlers(handlers ...interface{}) int {
	var i int
	for _, h := range handlers {
		switch handle := h.(type) {
		case ATMessageEventHandler:
			DefaultHandlers.ATMessage = handle
			i = i | 1<<30
		default:
		}
	}
	return i
}

// ParseAndHandle 处理回调事件
func ParseAndHandle(payload *WSPayload) error {
	// 指定类型的 handler
	if h, ok := eventParseFuncMap[payload.OPCode][payload.Type]; ok {
		return h(payload, payload.RawMessage)
	}
	return nil
}

var eventParseFuncMap = map[int]map[string]eventParseFunc{
	WSDispatchEvent: {
		"AT_MESSAGE_CREATE": atMessageHandler,
	},
}

func atMessageHandler(payload *WSPayload, message []byte) error {
	data := &Message{}
	if err := ParseData(message, data); err != nil {
		return err
	}
	if DefaultHandlers.ATMessage != nil {
		return DefaultHandlers.ATMessage(payload, data)
	}
	return nil
}
