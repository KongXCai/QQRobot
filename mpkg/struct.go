package mpkg

type IdiomData struct {
	Word        string `json:"word"`        //名称
	Derivation  string `json:"derivation"`  //出处
	Explanation string `json:"explanation"` //解释
	Pinyin      string `json:"pinyin"`      //拼音
}

// Config 定义了配置文件的结构
type Config struct {
	AppID uint64 `yaml:"appid"` //机器人的appid
	Token string `yaml:"token"` //机器人的token
}

// Message 消息结构体定义
type Message struct {
	// 消息ID
	ID string `json:"id"`
	// 子频道ID
	ChannelID string `json:"channel_id"`
	// 频道ID
	GuildID string `json:"guild_id"`
	// 内容
	Content string `json:"content"`
	// 发送时间
	Timestamp string `json:"timestamp"`
	// 消息编辑时间
	EditedTimestamp string `json:"edited_timestamp"`
	// 是否@all
	MentionEveryone bool `json:"mention_everyone"`
	// 消息发送方
	Author *User `json:"author"`
	// 消息发送方Author的member属性，只是部分属性
	Member *Member `json:"member"`
	// 附件
	Attachments []*MessageAttachment `json:"attachments"`
	// 结构化消息-embeds
	// 消息中的提醒信息(@)列表
	Mentions []*User `json:"mentions"`
	// ark 消息
	Ark *Ark `json:"ark"`
	// 私信消息
	DirectMessage bool `json:"direct_message"`
	// 子频道 seq，用于消息间的排序，seq 在同一子频道中按从先到后的顺序递增，不同的子频道之前消息无法排序
	SeqInChannel string `json:"seq_in_channel"`
	// 引用的消息
	// 私信场景下，该字段用来标识从哪个频道发起的私信
	SrcGuildID string `json:"src_guild_id"`
}

// MessageEmbedThumbnail embed 消息的缩略图对象
type MessageEmbedThumbnail struct {
	URL string `json:"url"`
}

// EmbedField Embed字段描述
type EmbedField struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

// MessageAttachment 附件定义
type MessageAttachment struct {
	URL string `json:"url"`
}

// User 用户
type User struct {
	ID               string `json:"id"`
	Username         string `json:"username"`
	Avatar           string `json:"avatar"`
	Bot              bool   `json:"bot"`
	UnionOpenID      string `json:"union_openid"`       // 特殊关联应用的 openid
	UnionUserAccount string `json:"union_user_account"` // 机器人关联的用户信息，与union_openid关联的应用是同一个
}

// Member 群成员
type Member struct {
	GuildID  string   `json:"guild_id"`
	JoinedAt string   `json:"joined_at"`
	Nick     string   `json:"nick"`
	User     *User    `json:"user"`
	Roles    []string `json:"roles"`
	OpUserID string   `json:"op_user_id,omitempty"`
}

// Ark 消息模版
type Ark struct {
	TemplateID int      `json:"template_id,omitempty"` // ark 模版 ID
	KV         []*ArkKV `json:"kv,omitempty"`          // ArkKV 数组
}

// ArkKV Ark 键值对
type ArkKV struct {
	Key   string    `json:"key,omitempty"`
	Value string    `json:"value,omitempty"`
	Obj   []*ArkObj `json:"obj,omitempty"`
}

// ArkObj Ark 对象
type ArkObj struct {
	ObjKV []*ArkObjKV `json:"obj_kv,omitempty"`
}

// ArkObjKV Ark 对象键值对
type ArkObjKV struct {
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"`
}

// MessageToCreate 发送消息结构体定义
type MessageToCreate struct {
	Content string `json:"content,omitempty"`
	Ark     *Ark   `json:"ark,omitempty"`
	// 要回复的消息id，为空是主动消息，公域机器人会异步审核，不为空是被动消息，公域机器人会校验语料
	MsgID   string `json:"msg_id,omitempty"`
	EventID string `json:"event_id,omitempty"` // 要回复的事件id, 逻辑同MsgID
}
