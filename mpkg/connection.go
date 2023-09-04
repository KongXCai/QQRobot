package mpkg

import (
	"context"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
)

// NewSessionManager 获得 session manager 实例
func NewSessionManager() *ChanManager {
	return New()
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
func (client *Client_struct) PostMessage(ctx context.Context, channelID string, msg *MessageToCreate) (*Message, error) {
	resp, err := client.restyClient.R().SetContext(ctx).
		SetResult(Message{}).
		SetPathParam("channel_id", channelID).
		SetBody(msg).
		Post("https://api.sgroup.qq.com/channels/{channel_id}/messages")
	if err != nil {
		return nil, err
	}

	return resp.Result().(*Message), nil
}

// Token 用于调用接口的 token 结构
type Token struct {
	AppID       uint64
	AccessToken string
	Type        string
}

func (tk *Token) ToStr() string {
	return fmt.Sprintf("%v.%s", tk.AppID, tk.AccessToken)
}

// New 创建本地session管理器
func New() *ChanManager {
	return &ChanManager{}
}

// ChanManager 默认的本地 session manager 实现
type ChanManager struct {
	sessionChan chan Session
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
const CTW = 2

// CalcInterval 根据并发要求，计算连接启动间隔
func CalcInterval(maxC uint32) time.Duration {
	if maxC == 0 {
		maxC = 1
	}
	f := math.Round(CTW / float64(maxC))
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
			PanicHandler(err, &session)
			l.sessionChan <- session
		}
	}()
	wsClient := NewWebsocket(session)
	if err := wsClient.Connect(); err != nil {
		log.Println(err)
		l.sessionChan <- session // 连接失败，丢回去队列排队重连
		return
	}
	var err error
	// 重连或者鉴权
	if session.ID != "" {
		err = wsClient.ReTry()
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
		currentSession := wsClient.GetSession()
		l.sessionChan <- *currentSession
		return
	}
}
