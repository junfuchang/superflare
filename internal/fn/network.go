package fn

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

var client = &http.Client{Timeout: 3 * time.Second}

const (
	maxJSONResponseBytes = 1 * 1024 * 1024
	maxHTMLResponseBytes = 4 * 1024 * 1024
)

var errResponseTooLarge = errors.New("response too large")

func GetJSON(url string, target interface{}) error {
	r, err := client.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if r.StatusCode < http.StatusOK || r.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("unexpected status: %d", r.StatusCode)
	}
	bodyBytes, err := readLimitedResponseBody(r.Body, maxJSONResponseBytes)
	if err != nil {
		return err
	}
	return json.Unmarshal(bodyBytes, target)
}

func GetHTML(url string) (result string, err error) {
	r, err := client.Get(url)
	if err != nil {
		return "", err
	}

	defer r.Body.Close()

	if r.StatusCode != http.StatusOK {
		return "", errors.New(strconv.Itoa(r.StatusCode))
	}

	bodyBytes, err := readLimitedResponseBody(r.Body, maxHTMLResponseBytes)
	if err != nil {
		return "", err
	}
	return string(bodyBytes), nil
}

func readLimitedResponseBody(reader io.Reader, limit int64) ([]byte, error) {
	bodyBytes, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(bodyBytes)) > limit {
		return nil, fmt.Errorf("%w: limit %d bytes", errResponseTooLarge, limit)
	}
	return bodyBytes, nil
}
