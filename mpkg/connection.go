package mpkg

import (
	"context"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"runtime"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/tencent-connect/botgo/dto"
	"github.com/tencent-connect/botgo/sessions/manager"
	"github.com/tencent-connect/botgo/websocket"
)

var defaultSessionManager SessionManager = New()

// NewSessionManager 获得 session manager 实例
func NewSessionManager() SessionManager {
	return defaultSessionManager
}

// WebsocketAP wss 接入点信息
type WebsocketAP struct {
	URL               string            `json:"url"`
	Shards            uint32            `json:"shards"`
	SessionStartLimit SessionStartLimit `json:"session_start_limit"`
}

// SessionStartLimit 链接频控信息
type SessionStartLimit struct {
	Total          uint32 `json:"total"`
	Remaining      uint32 `json:"remaining"`
	ResetAfter     uint32 `json:"reset_after"`
	MaxConcurrency uint32 `json:"max_concurrency"`
}

func (client *Client_struct) GetWSS(ctx context.Context) (*WebsocketAP, error) {
	resp, err := client.restyClient.R().SetContext(ctx).
		SetResult(WebsocketAP{}).
		Get("https://api.sgroup.qq.com/gateway/bot")
	if err != nil {
		return nil, err
	}

	return resp.Result().(*WebsocketAP), nil
}

// PostMessage 发消息
func (client *client_struct) PostMessage(ctx context.Context, channelID string, msg *mpkg.MessageToCreate) (*mpkg.Message, error) {
	resp, err := client.restyClient.R().SetContext(ctx).
		SetResult(mpkg.Message{}).
		SetPathParam("channel_id", channelID).
		SetBody(msg).
		Post("https://api.sgroup.qq.com/channels/{channel_id}/messages")
	if err != nil {
		return nil, err
	}

	return resp.Result().(*mpkg.Message), nil
}

// ShardConfig 连接的 shard 配置，ShardID 从 0 开始，ShardCount 最小为 1
type ShardConfig struct {
	ShardID    uint32
	ShardCount uint32
}

// Token 用于调用接口的 token 结构
type Token struct {
	AppID       uint64
	AccessToken string
	Type        string
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

// New 创建本地session管理器
func New() *ChanManager {
	return &ChanManager{}
}

// ChanManager 默认的本地 session manager 实现
type ChanManager struct {
	sessionChan chan Session
}

// SessionManager 接口，管理session
type SessionManager interface {
	// Start 启动连接，默认使用 apInfo 中的 shards 作为 shard 数量，如果有需要自己指定 shard 数，请修 apInfo 中的信息
	Start(apInfo *WebsocketAP, token *Token, intents *int) error
}

// Start 启动本地 session manager
func (l *ChanManager) Start(apInfo *WebsocketAP, token *Token, intents *int) error {
	startInterval := CalcInterval(apInfo.SessionStartLimit.MaxConcurrency)
	log.Printf("[ws/session/local] will start %d sessions and per session start interval is %s",
		apInfo.Shards, startInterval)

	// 按照shards数量初始化，用于启动连接的管理
	l.sessionChan = make(chan Session, apInfo.Shards)
	for i := uint32(0); i < apInfo.Shards; i++ {
		session := Session{
			URL:     apInfo.URL,
			Token:   *token,
			Intent:  *intents,
			LastSeq: 0,
			Shards: ShardConfig{
				ShardID:    i,
				ShardCount: apInfo.Shards,
			},
		}
		l.sessionChan <- session
	}

	for session := range l.sessionChan {
		// MaxConcurrency 代表的是每 5s 可以连多少个请求
		time.Sleep(startInterval)
		go l.newConnect(session)
	}
	return nil
}

// concurrencyTimeWindowSec 并发时间窗口，单位秒
const concurrencyTimeWindowSec = 2

// CalcInterval 根据并发要求，计算连接启动间隔
func CalcInterval(maxConcurrency uint32) time.Duration {
	if maxConcurrency == 0 {
		maxConcurrency = 1
	}
	f := math.Round(concurrencyTimeWindowSec / float64(maxConcurrency))
	if f == 0 {
		f = 1
	}
	return time.Duration(f) * time.Second
}

type Client_struct struct {
	AppID       uint64
	AccessToken string
	tokenType   string
	timeout     time.Duration
	restyClient *resty.Client // resty client 复用
}

var m_client Client_struct

func (client *Client_struct) Init(ID uint64, token string, duration time.Duration) *Client_struct {
	client.AppID = ID
	client.AccessToken = token
	client.timeout = duration
	client.tokenType = "Bot"
	client.restyClient = resty.New().
		SetTransport(createTransport(nil, 1000)). // 自定义 transport
		SetTimeout(client.timeout).
		SetAuthToken(fmt.Sprintf("%v.%s", client.AppID, client.AccessToken)).
		SetAuthScheme(client.tokenType)
	return client
}

func NewClient(ID uint64, token string, duration time.Duration) *Client_struct {
	return m_client.Init(ID, token, duration)
}

func createTransport(localAddr net.Addr, idleConns int) *http.Transport {
	dialer := &net.Dialer{
		Timeout:   60 * time.Second,
		KeepAlive: 60 * time.Second,
	}
	if localAddr != nil {
		dialer.LocalAddr = localAddr
	}
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          idleConns,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConnsPerHost:   idleConns,
		MaxConnsPerHost:       idleConns,
	}
}

// newConnect 启动一个新的连接，如果连接在监听过程中报错了，或者被远端关闭了链接，需要识别关闭的原因，能否继续 resume
// 如果能够 resume，则往 sessionChan 中放入带有 sessionID 的 session
// 如果不能，则清理掉 sessionID，将 session 放入 sessionChan 中
// session 的启动，交给 start 中的 for 循环执行，session 不自己递归进行重连，避免递归深度过深
func (l *ChanManager) newConnect(session Session) {
	defer func() {
		// panic 留下日志，放回 session
		if err := recover(); err != nil {
			websocket.PanicHandler(err, &session)
			l.sessionChan <- session
		}
	}()
	wsClient := websocket.ClientImpl.New(session)
	if err := wsClient.Connect(); err != nil {
		log.Println(err)
		l.sessionChan <- session // 连接失败，丢回去队列排队重连
		return
	}
	var err error
	// 如果 session id 不为空，则执行的是 resume 操作，如果为空，则执行的是 identify 操作
	if session.ID != "" {
		err = wsClient.Resume()
	} else {
		// 初次鉴权
		err = wsClient.Identify()
	}
	if err != nil {
		log.Printf("[ws/session] Identify/Resume err %+v", err)
		return
	}
	if err := wsClient.Listening(); err != nil {
		log.Printf("[ws/session] Listening err %+v", err)
		currentSession := wsClient.Session()
		// 对于不能够进行重连的session，需要清空 session id 与 seq
		if manager.CanNotResume(err) {
			currentSession.ID = ""
			currentSession.LastSeq = 0
		}
		// 一些错误不能够鉴权，比如机器人被封禁，这里就直接退出了
		if manager.CanNotIdentify(err) {
			msg := fmt.Sprintf("can not identify because server return %+v, so process exit", err)
			log.Printf(msg)
			panic(msg) // 当机器人被下架，或者封禁，将不能再连接，所以 panic
		}
		// 将 session 放到 session chan 中，用于启动新的连接，当前连接退出
		l.sessionChan <- *currentSession
		return
	}
}

// PanicHandler 处理websocket场景的 panic ，打印堆栈
func PanicHandler(e interface{}, session *dto.Session) {
	buf := make([]byte, PanicBufLen)
	buf = buf[:runtime.Stack(buf, false)]
	log.Errorf("[PANIC]%s\n%v\n%s\n", session, e, buf)
}
