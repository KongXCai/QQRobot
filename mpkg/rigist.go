package mpkg

// DefaultHandlers 默认的 handler 结构，管理所有支持的 handler 类型
var DefaultHandlers struct {
	// Ready       ReadyHandler
	// ErrorNotify ErrorNotifyHandler
	// Plain       PlainEventHandler

	// Guild       GuildEventHandler
	// GuildMember GuildMemberEventHandler
	// Channel     ChannelEventHandler

	// Message             MessageEventHandler
	// MessageReaction     MessageReactionEventHandler
	ATMessage ATMessageEventHandler
	// DirectMessage       DirectMessageEventHandler
	// MessageAudit        MessageAuditEventHandler
	// MessageDelete       MessageDeleteEventHandler
	// PublicMessageDelete PublicMessageDeleteEventHandler
	// DirectMessageDelete DirectMessageDeleteEventHandler

	// Audio AudioEventHandler

	// Thread     ThreadEventHandler
	// Post       PostEventHandler
	// Reply      ReplyEventHandler
	// ForumAudit ForumAuditEventHandler

	// Interaction InteractionEventHandler
}

// ATMessageEventHandler at 机器人消息事件 handler
type ATMessageEventHandler func(event *WSPayload, data *Message) error

// RegisterHandlers 注册事件回调，并返回 intent 用于 websocket 的鉴权
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
