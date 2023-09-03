package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"mpkg"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tencent-connect/botgo"
	yaml "gopkg.in/yaml.v2"
)

// Config 定义了配置文件的结构
type Config struct {
	AppID uint64 `yaml:"appid"` //机器人的appid
	Token string `yaml:"token"` //机器人的token
}

type idiomData struct {
	Word        string `json:"word"`
	Derivation  string `json:"derivation"`
	Explanation string `json:"explanation"`
	Pinyin      string `json:"pinyin"`
}

var config Config
var idioms []idiomData
var ctx context.Context
var gameIsRunning bool = false
var firstWord string = ""
var lastWord string = ""

type idiomsMap map[string]([]*idiomData)
type idiomsSet map[string]bool
type idiomCount map[string]int

var idiomsIndex idiomsMap
var isSame idiomsSet
var idiomDatabase idiomsSet
var idiomCnt idiomCount
var tm *time.Timer

// 第一步： 获取机器人的配置信息，即机器人的appid和token
func init() {
	content, err := os.ReadFile("config.yaml")
	if err != nil {
		log.Println("读取配置文件出错， err = ", err)
		os.Exit(1)
	}

	err = yaml.Unmarshal(content, &config)
	if err != nil {
		log.Println("解析配置文件出错， err = ", err)
		os.Exit(1)
	}
	log.Println(config)

	// 读取JSON文件
	filePath := "data.json"
	jsonData, err := os.ReadFile(filePath)
	if err != nil {
		log.Println("无法读取成语库文件 err = ", err)
		os.Exit(1)
	}

	// 解析JSON数据
	err = json.Unmarshal(jsonData, &idioms)
	if err != nil {
		log.Println("无法解析成语库文件 err = ", err)
		os.Exit(1)
	}
	idiomsIndex = make(idiomsMap)
	idiomDatabase = make(idiomsSet)
	// 给成语库建立索引
	for i, _ := range idioms {
		firstChar := getFirstChineseChar(idioms[i].Word)
		idiomsIndex[firstChar] = append(idiomsIndex[firstChar], &idioms[i])
		idiomDatabase[idioms[i].Word] = true
	}
	log.Println("成语库加载成功")
}

func main() {
	// //第二步：生成token，用于校验机器人的身份信息
	token := &mpkg.Token{
		AppID:       config.AppID,
		AccessToken: config.Token,
		Type:        "Bot",
	}
	// //第三步：获取操作机器人的API对象
	// api = botgo.NewOpenAPI(token).WithTimeout(3 * time.Second)
	// 第四步：获取websocket
	// ws, err := api.WS(ctx, nil, "")

	// 获取context
	ctx = context.Background()
	// 初始化http连接
	m_client := mpkg.NewClient(config.AppID, config.Token, 3*time.Second)
	// 通过http获取webSocket连接地址
	ws, err := m_client.GetWSS(ctx)
	if err != nil {
		log.Fatalln("websocket错误， err = ", err)
		os.Exit(1)
	} else {
		log.Printf("%+v, err:%v", ws, err)
	}

	var atMessage mpkg.ATMessageEventHandler = atMessageEventHandler
	println(atMessage)
	intent := mpkg.RegisterHandlers(atMessage) // 注册socket消息处理
	fmt.Println(intent)
	botgo.NewSessionManager().Start(ws, token, &intent) // 启动socket监听
}

// atMessageEventHandler 处理 @机器人 的消息
func atMessageEventHandler(event *mpkg.WSPayload, data *mpkg.Message) error {
	if gameIsRunning {
		tm.Reset(60 * time.Second)
	}
	message_content := data.Content[strings.Index(data.Content, ">")+2:]
	if message_content == "/成语接龙" {
		if gameIsRunning {
			m_client.PostMessage(ctx, data.ChannelID, &mpkg.MessageToCreate{MsgID: data.ID, Content: "游戏正在进行中！"})
		} else {
			isSame = make(idiomsSet)
			idiomCnt = make(idiomCount)
			for key, value := range idiomsIndex {
				idiomCnt[key] = len(value)
			}
			gameIsRunning = true
			tm = time.NewTimer(60 * time.Second)
			go timeout_handle(tm, data)
			m_client.PostMessage(ctx, data.ChannelID, &mpkg.MessageToCreate{MsgID: data.ID, Content: "游戏开始，请说一个四字成语。"})
		}
	} else if gameIsRunning && message_content == "/认输" {
		//直接重置timer时间，让timer快点触发
		tm.Reset(time.Microsecond)
	} else if gameIsRunning && isFourChineseCharacters(message_content) {
		//先判断是否是成语
		_, yes := idiomDatabase[message_content]
		if yes {
			//如果是成语判断成语是不是符合要求，即第一个字是上一个成语的最后一个字
			temp_firstWord := getFirstChineseChar(message_content)
			if lastWord != "" && temp_firstWord != lastWord {
				//第一个字不符合要求
				m_client.PostMessage(ctx, data.ChannelID, &mpkg.MessageToCreate{MsgID: data.ID, Content: "需要说一个第一个汉字为 \"" + lastWord + "\" 的成语!"})
			} else {
				//符合要求需要判断一下成语是不是说过
				_, yes = isSame[message_content]
				if yes {
					//如果是说过的，提示
					m_client.PostMessage(ctx, data.ChannelID, &mpkg.MessageToCreate{MsgID: data.ID, Content: "这个成语讲过啦，请重新说一个!"})
				} else {
					//是一个符合要求的成语
					isSame[message_content] = true
					idiomCnt[temp_firstWord]--
					lastWord = getLastChineseChar(message_content)
					_, yes = idiomDatabase[message_content]
					if yes && idiomCnt[lastWord] > 0 {
						idiomCnt[lastWord]--
						// 从列表中随机选择一个下标
						index := rand.Intn(len(idiomsIndex[lastWord]))
						new_idiom := idiomsIndex[lastWord][index].Word
						_, t := isSame[new_idiom]
						for t {
							// 从列表中随机选择一个下标
							index = rand.Intn(len(idiomsIndex[lastWord]))
							new_idiom = idiomsIndex[lastWord][index].Word
							_, t = isSame[new_idiom]
						}
						isSame[new_idiom] = true
						lastWord = getLastChineseChar(new_idiom)
						m_client.PostMessage(ctx, data.ChannelID, &mpkg.MessageToCreate{MsgID: data.ID, Content: new_idiom})
					} else {
						//词库里已经没有最后一个字开头的成语了，认输！
						m_client.PostMessage(ctx, data.ChannelID, &mpkg.MessageToCreate{MsgID: data.ID, Content: "太厉害啦！我找不到合适的成语接着往下玩了，拜了拜了！"})
						gameIsRunning = false
						firstWord = ""
						lastWord = ""
					}
				}
			}
		} else {
			//输入的不是成语提示
			m_client.PostMessage(ctx, data.ChannelID, &mpkg.MessageToCreate{MsgID: data.ID, Content: "这可能不是一个成语，请再重新说一个!"})
		}
	} else if gameIsRunning && !isFourChineseCharacters(message_content) {
		m_client.PostMessage(ctx, data.ChannelID, &mpkg.MessageToCreate{MsgID: data.ID, Content: "请告诉我一个四字的成语喔~"})
	} else {
		m_client.PostMessage(ctx, data.ChannelID, &mpkg.MessageToCreate{MsgID: data.ID, Content: "我是菜菜机器人，你可以跟我玩成语接龙游戏，也可以查成语。"})
	}
	return nil
}

// 获取中文字符串的第一个字符
func getFirstChineseChar(s string) string {
	r, _ := utf8.DecodeRuneInString(s)
	return string(r)
}

// 获取中文字符串的最后一个字符
func getLastChineseChar(s string) string {
	_, size := utf8.DecodeLastRuneInString(s)
	return s[len(s)-size:]
}

func isFourChineseCharacters(s string) bool {
	// 获取字符串的字节数
	byteCount := len(s)

	// 获取字符串的Unicode字符数
	charCount := utf8.RuneCountInString(s)

	// 判断字节数和字符数是否都为4
	return byteCount == 12 && charCount == 4
}

func timeout_handle(timer *time.Timer, data *mpkg.Message) {
	<-timer.C
	gameIsRunning = false
	firstWord = ""
	lastWord = ""
	m_client.PostMessage(ctx, data.ChannelID, &mpkg.MessageToCreate{MsgID: data.ID, Content: "欢迎下次继续挑战，本次成语接龙游戏已结束，再见！"})
}
