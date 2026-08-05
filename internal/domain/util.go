package domain

import (
	"crypto/rand"
	"encoding/hex"
	"math/big"
	"time"
)

// NowMillis 返回当前毫秒时间戳
func NowMillis() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

// GenerateRandomString 生成指定长度的随机字符串（用于生成 ID、Key 等）
func GenerateRandomString(n int) string {
	const letters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	ret := make([]byte, n)
	for i := 0; i < n; i++ {
		num, _ := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		ret[i] = letters[num.Int64()]
	}
	return string(ret)
}

// GenerateUUID 生成简单的随机 UUID 字符串
func GenerateUUID() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}
