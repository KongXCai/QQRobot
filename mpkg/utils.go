package mpkg

import (
	"encoding/json"
	"log"
	"runtime"
	"unicode/utf8"

	"github.com/tidwall/gjson"
)

// ParseData 解析数据
func ParseData(message []byte, target interface{}) error {
	data := gjson.Get(string(message), "d")
	return json.Unmarshal([]byte(data.String()), target)
}

// PanicHandler 处理websocket场景的 panic ，打印堆栈
func PanicHandler(e interface{}, session *Session) {
	buf := make([]byte, 1024)
	buf = buf[:runtime.Stack(buf, false)]
	log.Printf("[PANIC]%s\n%v\n%s\n", session, e, buf)
}

// 获取中文字符串的第一个字符
func GetFirstChineseChar(s string) string {
	r, _ := utf8.DecodeRuneInString(s)
	return string(r)
}

// 获取中文字符串的最后一个字符
func GetLastChineseChar(s string) string {
	_, size := utf8.DecodeLastRuneInString(s)
	return s[len(s)-size:]
}

func IsFourChineseCharacters(s string) bool {
	// 获取字符串的字节数
	byteCount := len(s)

	// 获取字符串的Unicode字符数
	charCount := utf8.RuneCountInString(s)

	// 判断字节数和字符数是否都为4
	return byteCount == 12 && charCount == 4
}
