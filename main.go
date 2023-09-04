package main

import (
	"context"
	"demo/mpkg"
	"encoding/json"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v2"
)

// 根据成语首字母建立哈希表，所有首字母相同的成语存储在一个列表中
type idiomsMap map[string]([]*mpkg.IdiomData)
type idiomsSet map[string]bool
type idiomsToPtr map[string]*mpkg.IdiomData
type idiomCount map[string]int

var config mpkg.Config
var ctx context.Context

// 接龙游戏的定时timer
var tm *time.Timer

// 存放成语的列表，索引结构中存放的是指针
var idioms []mpkg.IdiomData

// 游戏状态标志位，表示游戏是否在进行中
var gameIsRunning bool = false

// 目前成语的最后一个字，也就是下一个成语应该出现的第一个字
var lastWord string = ""

// 成语索引哈希表
var idiomsIndex idiomsMap

// 记录成语以识别是否出现过
var isSame idiomsSet

// 全量成语的set,用来快速找到指定成语
var idiomDatabase idiomsToPtr

// 纪录每个列表中还剩多少没出现过的成语，以判断是否应该结束游戏
var idiomCnt idiomCount

// 获取机器人的配置信息和初始化成语词库
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
	idiomDatabase = make(idiomsToPtr)
	// 给成语库建立索引
	for i, _ := range idioms {
		firstChar := mpkg.GetFirstChineseChar(idioms[i].Word)
		idiomsIndex[firstChar] = append(idiomsIndex[firstChar], &idioms[i])
		idiomDatabase[idioms[i].Word] = &(idioms[i])
	}
	log.Println("成语库加载成功")
}

var m_client *mpkg.Client_struct

func main() {
	// 获取context
	ctx = context.Background()
	// 初始化http连接
	m_client = mpkg.NewClient(config.AppID, config.Token, 3*time.Second)
	// 通过http获取webSocket连接地址
	ws, err := m_client.GetWSS(ctx)
	if err != nil {
		log.Fatalln("websocket错误， err = ", err)
		os.Exit(1)
	} else {
		log.Printf("%+v, err:%v", ws, err)
	}
	// 注册@消息的回调函数
	var atMessage mpkg.ATMessageEventHandler = atMessageEventHandler
	intent := mpkg.RegisterHandlers(atMessage)                                                                            // 注册socket消息处理
	mpkg.NewSessionManager().Start(ws, &mpkg.Token{AppID: config.AppID, AccessToken: config.Token, Type: "Bot"}, &intent) // 启动socket监听
}

// 处理 @机器人消息的回调函数
func atMessageEventHandler(event *mpkg.WSPayload, data *mpkg.Message) error {
	if gameIsRunning {
		// 收到消息就刷新timer
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
	} else if strings.HasPrefix(message_content, "/解释成语") {
		exp_idiom := message_content[strings.Index(message_content, " "):]
		if len(exp_idiom) > 1 && mpkg.IsFourChineseCharacters(exp_idiom[1:]) {
			_, yes := idiomDatabase[exp_idiom[1:]]
			if yes {
				to_exp_idiom := idiomDatabase[exp_idiom[1:]]
				// out_content := to_exp_idiom.Word + ": " + to_exp_idiom.Pinyin + "\n出处：" + to_exp_idiom.Derivation + ":\n解释：" + to_exp_idiom.Explanation
				m_client.PostMessage(ctx, data.ChannelID, &mpkg.MessageToCreate{MsgID: data.ID, Ark: mpkg.ArkMessage(to_exp_idiom)})
			} else {
				m_client.PostMessage(ctx, data.ChannelID, &mpkg.MessageToCreate{MsgID: data.ID, Content: "我也不认识。"})
			}
		} else {
			println(exp_idiom[1:])
			m_client.PostMessage(ctx, data.ChannelID, &mpkg.MessageToCreate{MsgID: data.ID, Content: "请输入正确的四字成语！"})
		}
	} else if gameIsRunning && mpkg.IsFourChineseCharacters(message_content) {
		//先判断是否是成语
		_, yes := idiomDatabase[message_content]
		if yes {
			//如果是成语判断成语是不是符合要求，即第一个字是上一个成语的最后一个字
			temp_firstWord := mpkg.GetFirstChineseChar(message_content)
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
					lastWord = mpkg.GetLastChineseChar(message_content)
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
						lastWord = mpkg.GetLastChineseChar(new_idiom)
						m_client.PostMessage(ctx, data.ChannelID, &mpkg.MessageToCreate{MsgID: data.ID, Content: new_idiom})
					} else {
						//词库里已经没有最后一个字开头的成语了，认输！
						m_client.PostMessage(ctx, data.ChannelID, &mpkg.MessageToCreate{MsgID: data.ID, Content: "太厉害啦！我找不到合适的成语接着往下玩了，拜了拜了！"})
						gameIsRunning = false
						lastWord = ""
					}
				}
			}
		} else {
			//输入的不是成语提示
			m_client.PostMessage(ctx, data.ChannelID, &mpkg.MessageToCreate{MsgID: data.ID, Content: "这可能不是一个成语，请再重新说一个!"})
		}
	} else if gameIsRunning && !mpkg.IsFourChineseCharacters(message_content) {
		m_client.PostMessage(ctx, data.ChannelID, &mpkg.MessageToCreate{MsgID: data.ID, Content: "请告诉我一个四字的成语喔~"})
	} else {
		m_client.PostMessage(ctx, data.ChannelID, &mpkg.MessageToCreate{MsgID: data.ID, Content: "我是菜菜机器人，你可以跟我玩成语接龙游戏，也可以查成语。"})
	}
	return nil
}

// timer超时没响应自动结束游戏的回调函数
func timeout_handle(timer *time.Timer, data *mpkg.Message) {
	<-timer.C
	gameIsRunning = false
	lastWord = ""
	m_client.PostMessage(ctx, data.ChannelID, &mpkg.MessageToCreate{MsgID: data.ID, Content: "欢迎下次继续挑战，本次成语接龙游戏已结束，再见！"})
}
