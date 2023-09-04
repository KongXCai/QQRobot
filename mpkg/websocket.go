package mpkg

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	wss "github.com/gorilla/websocket"
)

var ResumeSignal syscall.Signal

type messageChan chan *WSPayload
type closeErrorChan chan error

// Client websocket 连接客户端
type Websocket_clt struct {
	version         int
	conn            *wss.Conn
	messageQueue    messageChan
	session         *Session
	user            *WSUser
	closeChan       closeErrorChan
	heartBeatTicker *time.Ticker // 用于维持定时心跳
}

// WSPayload websocket 消息结构
type WSPayload struct {
	WSPayloadBase
	Data       interface{} `json:"d,omitempty"`
	RawMessage []byte      `json:"-"` // 原始的 message 数据
}

// WSPayloadBase 基础消息结构，排除了 data
type WSPayloadBase struct {
	OPCode int    `json:"op"`
	Seq    uint32 `json:"s,omitempty"`
	Type   string `json:"t,omitempty"`
}

// Session 连接的 session 结构，包括链接的所有必要字段
type Session struct {
	ID      string
	URL     string
	Token   Token
	Intent  int
	LastSeq uint32
	Shards  ShardConfig
}

// ShardConfig 连接的 shard 配置，ShardID 从 0 开始，ShardCount 最小为 1
type ShardConfig struct {
	ShardID    uint32
	ShardCount uint32
}

// WSUser 当前连接的用户信息
type WSUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Bot      bool   `json:"bot"`
}

// WSIdentityData 鉴权数据
type WSIdentityData struct {
	Token      string   `json:"token"`
	Intents    int      `json:"intents"`
	Shard      []uint32 `json:"shard"` // array of two integers (shard_id, num_shards)
	Properties struct {
		Os      string `json:"$os,omitempty"`
		Browser string `json:"$browser,omitempty"`
		Device  string `json:"$device,omitempty"`
	} `json:"properties,omitempty"`
}

// WSReadyData ready，鉴权后返回
type WSReadyData struct {
	Version   int    `json:"version"`
	SessionID string `json:"session_id"`
	User      struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Bot      bool   `json:"bot"`
	} `json:"user"`
	Shard []uint32 `json:"shard"`
}

// WSHelloData hello 返回
type WSHelloData struct {
	HeartbeatInterval int `json:"heartbeat_interval"`
}

// WSResumeData 重连数据
type WSResumeData struct {
	Token     string `json:"token"`
	SessionID string `json:"session_id"`
	Seq       uint32 `json:"seq"`
}

// WS OPCode
const (
	WSDispatchEvent int = iota
	WSHeartbeat
	WSIdentity
	_ // Presence Update
	_ // Voice State Update
	_
	WSReTry
	WSReconnect
	_ // Request Guild Members
	WSInvalidSession
	WSHello
	WSHeartbeatAck
	HTTPCallbackAck
)

// opMeans op 对应的含义字符串标识
var opMeans = map[int]string{
	WSDispatchEvent:  "Event",
	WSHeartbeat:      "Heartbeat",
	WSIdentity:       "Identity",
	WSReTry:          "ReTry",
	WSReconnect:      "Reconnect",
	WSInvalidSession: "InvalidSession",
	WSHello:          "Hello",
	WSHeartbeatAck:   "HeartbeatAck",
}

// OPMeans 返回 op 含义
func GetOpMeans(op int) string {
	means, ok := opMeans[op]
	if !ok {
		means = "unknown"
	}
	return means
}

// New 创建一个新的ws实例，需要传递 session 对象
func NewWebsocket(session Session) *Websocket_clt {
	return &Websocket_clt{
		messageQueue:    make(messageChan, 2000),
		session:         &session,
		closeChan:       make(closeErrorChan, 10),
		heartBeatTicker: time.NewTicker(60 * time.Second), // 先给一个默认 ticker，在收到 hello 包之后，会 reset
	}
}

// 连接到 wss 地址
func (c *Websocket_clt) Connect() error {
	if c.session.URL == "" {
		return errors.New("websocket url is invalid")
	}

	var err error
	c.conn, _, err = wss.DefaultDialer.Dial(c.session.URL, nil)
	if err != nil {
		log.Printf("%s, connect err: %v", c.session, err)
		return err
	}
	log.Printf("%s, url %s, connected", c.session, c.session.URL)
	return nil
}

// 鉴权
func (c *Websocket_clt) Identify() error {
	// 避免传错 intent
	if c.session.Intent == 0 {
		c.session.Intent = 1
	}
	payload := &WSPayload{
		Data: &WSIdentityData{
			Token:   c.session.Token.ToStr(),
			Intents: c.session.Intent,
			Shard: []uint32{
				c.session.Shards.ShardID,
				c.session.Shards.ShardCount,
			},
		},
	}
	payload.OPCode = WSIdentity
	return c.SendMessage(payload)
}

// 拉取 session 信息，包括 token，shard，seq 等
func (c *Websocket_clt) GetSession() *Session {
	return c.session
}

// 重连
func (c *Websocket_clt) ReTry() error {
	payload := &WSPayload{
		Data: &WSResumeData{
			Token:     c.session.Token.ToStr(),
			SessionID: c.session.ID,
			Seq:       c.session.LastSeq,
		},
	}
	payload.OPCode = WSReTry // 内嵌结构体字段，单独赋值
	return c.SendMessage(payload)
}

// 监听websocket事件
func (c *Websocket_clt) Listening() error {
	defer c.Close()
	// reading message
	go c.readMessageToQueue()
	// read message from queue and handle,in goroutine to avoid business logic block closeChan and heartBeatTicker
	go c.listenMessageAndHandle()

	// 接收 resume signal
	resumeSignal := make(chan os.Signal, 1)
	if ResumeSignal >= syscall.SIGHUP {
		signal.Notify(resumeSignal, ResumeSignal)
	}

	// handler message
	for {
		select {
		case <-resumeSignal: // 使用信号量控制连接立即重连
			log.Printf("%s, received resumeSignal signal", c.session)
			return errors.New("need reconnect")
		case err := <-c.closeChan:
			log.Printf("%s Listening stop. err is %v", c.session, err)
			if wss.IsCloseError(err, 4914, 4915) {
				err = errors.New(err.Error())
			}
			if wss.IsUnexpectedCloseError(err, 4009) {
				err = errors.New(err.Error())
			}
			return err
		case <-c.heartBeatTicker.C:
			log.Printf("%s listened heartBeat", c.session)
			heartBeatEvent := &WSPayload{
				WSPayloadBase: WSPayloadBase{
					OPCode: WSHeartbeat,
				},
				Data: c.session.LastSeq,
			}
			// 不处理错误，Write 内部会处理，如果发生发包异常，会通知主协程退出
			_ = c.SendMessage(heartBeatEvent)
		}
	}
}

// 发送数据
func (c *Websocket_clt) SendMessage(message *WSPayload) error {
	m, _ := json.Marshal(message)
	log.Printf("%s write %s message, %v", c.session, GetOpMeans(message.OPCode), string(m))

	if err := c.conn.WriteMessage(wss.TextMessage, m); err != nil {
		log.Printf("%s WriteMessage failed, %v", c.session, err)
		c.closeChan <- err
		return err
	}
	return nil
}

// 关闭连接
func (c *Websocket_clt) Close() {
	if err := c.conn.Close(); err != nil {
		log.Printf("%s, close conn err: %v", c.session, err)
	}
	c.heartBeatTicker.Stop()
}

func (c *Websocket_clt) readMessageToQueue() {
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			log.Printf("%s read message failed, %v, message %s", c.session, err, string(message))
			close(c.messageQueue)
			c.closeChan <- err
			return
		}
		payload := &WSPayload{}
		if err := json.Unmarshal(message, payload); err != nil {
			log.Printf("%s json failed, %v", c.session, err)
			continue
		}
		payload.RawMessage = message
		log.Printf("%s receive %s message, %s", c.session, GetOpMeans(payload.OPCode), string(message))
		// 处理内置的一些事件，如果处理成功，则这个事件不再投递给业务
		if c.isHandleBuildIn(payload) {
			continue
		}
		c.messageQueue <- payload
	}
}

func (c *Websocket_clt) listenMessageAndHandle() {
	defer func() {
		// panic，一般是由于业务自己实现的 handle 不完善导致
		// 打印日志后，关闭这个连接，进入重连流程
		if err := recover(); err != nil {
			PanicHandler(err, c.session)
			c.closeChan <- fmt.Errorf("panic: %v", err)
		}
	}()
	for payload := range c.messageQueue {
		if payload.Seq > 0 {
			c.session.LastSeq = payload.Seq
		}
		// ready 事件需要特殊处理
		if payload.Type == "READY" {
			c.readyHandler(payload)
			continue
		}
		// 解析具体事件，并投递给业务注册的 handler
		if err := ParseAndHandle(payload); err != nil {
			log.Printf("%s parseAndHandle failed, %v", c.session, err)
		}
	}
	log.Printf("%s message queue is closed", c.session)
}

// isHandleBuildIn 内置的事件处理，处理那些不需要业务方处理的事件
// return true 的时候说明事件已经被处理了
func (c *Websocket_clt) isHandleBuildIn(payload *WSPayload) bool {
	switch payload.OPCode {
	case WSHello: // 接收到 hello 后需要开始发心跳
		c.startHeartBeatTicker(payload.RawMessage)
	case WSHeartbeatAck: // 心跳 ack 不需要业务处理
	case WSReconnect: // 达到连接时长，需要重新连接，此时可以通过 resume 续传原连接上的事件
		c.closeChan <- errors.New("need reconnect")
	case WSInvalidSession: // 无效的 sessionLog，需要重新鉴权
		c.closeChan <- errors.New("invalid session")
	default:
		return false
	}
	return true
}

// startHeartBeatTicker 启动定时心跳
func (c *Websocket_clt) startHeartBeatTicker(message []byte) {
	helloData := &WSHelloData{}
	if err := ParseData(message, helloData); err != nil {
		log.Printf("%s hello data parse failed, %v, message %v", c.session, err, message)
	}
	// 根据 hello 的回包，重新设置心跳的定时器时间
	c.heartBeatTicker.Reset(time.Duration(helloData.HeartbeatInterval) * time.Millisecond)
}

// readyHandler 针对ready返回的处理，需要记录 sessionID 等相关信息
func (c *Websocket_clt) readyHandler(payload *WSPayload) {
	readyData := &WSReadyData{}
	if err := ParseData(payload.RawMessage, readyData); err != nil {
		log.Printf("%s parseReadyData failed, %v, message %v", c.session, err, payload.RawMessage)
	}
	c.version = readyData.Version
	// 基于 ready 事件，更新 session 信息
	c.session.ID = readyData.SessionID
	c.session.Shards.ShardID = readyData.Shard[0]
	c.session.Shards.ShardCount = readyData.Shard[1]
	c.user = &WSUser{
		ID:       readyData.User.ID,
		Username: readyData.User.Username,
		Bot:      readyData.User.Bot,
	}
}
