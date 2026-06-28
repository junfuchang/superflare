package data

import (
	"bytes"
	"crypto/md5" //#nosec
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

var jsonStringify = func(data interface{}) (string, error) {
	buff := bytes.NewBuffer([]byte{})
	encoder := json.NewEncoder(buff)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(data)
	if err != nil {
		return "", fmt.Errorf("json stringify failed: %w", err)
	}
	return strings.TrimSpace(buff.String()), nil
}

func MaskTextWithStars(input string) string {
	chars := []rune(input)
	mask := chars
	count := len(chars)
	for i := 0; i < count; i++ {
		if i != 0 && i != count-1 {
			mask[i] = '*'
		}
	}
	return string(mask)
}

func GenerateRandomString(size int) string {
	id := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, id); err != nil {
		data := []byte(time.Now().String())
		/* #nosec */
		hash := md5.Sum(data)
		return hex.EncodeToString(hash[0:size])
	}
	return hex.EncodeToString(id)[:size]
}

func Base64EncodeUrl(input string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(input))
	return url.QueryEscape(encoded)
}

func Base64DecodeUrl(input string) ([]byte, error) {
	unescaped, err := url.QueryUnescape(input)
	if err != nil {
		return []byte{}, err
	}
	return base64.StdEncoding.DecodeString(unescaped)
}
