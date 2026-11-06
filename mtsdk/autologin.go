package mtsdk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const (
	// DefaultGameChannelID 官服渠道号，抽卡接口与 auto_login 都需要。
	DefaultGameChannelID = 5
	// DefaultReyunChannelID 官服 PC 端的 cl.bin 内容。
	DefaultReyunChannelID = "gwpc"

	accountBaseURL     = "https://gf2-zoneinfo.sunborngame.com/"
	autoLoginURL       = accountBaseURL + "account/auto_login"
	gachaRecordBaseURL = "https://gf2-gacha-record.sunborngame.com/list"

	unityUserAgent = "UnityPlayer/2019.4.40f1 (UnityWebRequest/1.0, libcurl/7.80.0-DEV)"
	unityVersion   = "2019.4.40f1"

	requestTimeout = 20 * time.Second
)

// LoginData 是 account/auto_login 返回的账号信息。
type LoginData struct {
	AccessToken    string `json:"access_token"`
	UcAccountToken string `json:"uc_account_token"`
	Uid            int64  `json:"uid"`
	PhoneNumber    string `json:"phone_number"`
}

// APIError 是接口返回的 code != 0。
type APIError struct {
	Code    int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s(Code %d)", e.Message, e.Code)
}

// IsTokenError 判断错误是不是「令牌已失效」。
//
// 抽卡接口对失效令牌返回 {"code":-1,"message":"token err"}，
// 账号接口则返回 {"code":10004,"msg":"访问令牌无效"}。
func IsTokenError(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}

	message := strings.ToLower(apiErr.Message)
	return apiErr.Code == http.StatusUnauthorized ||
		strings.Contains(message, "token") ||
		strings.Contains(apiErr.Message, "令牌") ||
		strings.Contains(apiErr.Message, "凭证")
}

// GachaURL 拼出抽卡记录接口地址。
// 官方客户端在 query 里固定写 type_id=3,真正的池子 id 放在表单里。
func GachaURL(channel int64) string {
	return fmt.Sprintf("%s?game_channel_id=%d&type_id=3", gachaRecordBaseURL, channel)
}

// ParseChannel 从抽卡链接里取出 game_channel_id。
func ParseChannel(gachaURL string) (int64, bool) {
	parsed, err := url.Parse(gachaURL)
	if err != nil {
		return 0, false
	}

	value := parsed.Query().Get("game_channel_id")
	if value == "" {
		return 0, false
	}

	channel, err := strconv.ParseInt(value, 10, 64)
	if err != nil || channel <= 0 {
		return 0, false
	}

	return channel, true
}

// AutoLogin 用本地保存的令牌换取该账号当前有效的令牌。
//
// 关键点：本地令牌可能已经不是「当前会话」令牌(抽卡接口会返回 token err)，
// 但 account/auto_login 仍然接受它，并返回可用的新令牌。
func AutoLogin(token, deviceNum string, channel int64, reyunChannelID string) (*LoginData, error) {
	if token == "" {
		return nil, errors.New("本地令牌为空")
	}
	if deviceNum == "" {
		return nil, errors.New("MTLocalData 中没有 device_num,无法调用 account/auto_login")
	}
	if channel <= 0 {
		channel = DefaultGameChannelID
	}
	if reyunChannelID == "" {
		reyunChannelID = DefaultReyunChannelID
	}

	payload := map[string]interface{}{
		"game_channel_id":  channel,
		"device_num":       deviceNum,
		"mac":              "",
		"os":               3,
		"language":         "cn",
		"reyun_channel_id": reyunChannelID,
	}

	resp, err := postJSON(autoLoginURL, token, payload)
	if err != nil {
		return nil, err
	}

	var data LoginData
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return nil, errors.Wrap(err, "account/auto_login 返回的数据无法解析")
	}
	if data.AccessToken == "" {
		return nil, errors.New("account/auto_login 没有返回 access_token")
	}

	return &data, nil
}

// ProbeToken 用一次最小请求确认令牌是否能读取抽卡记录。
func ProbeToken(token string, channel int64) error {
	if token == "" {
		return &APIError{Code: http.StatusUnauthorized, Message: "令牌为空"}
	}

	form := url.Values{}
	form.Set("type_id", "3")

	body, err := doRequest(GachaURL(channel), token, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}

	var resp apiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return errors.Wrap(err, "抽卡接口返回的内容无法解析")
	}
	if resp.Code != 0 {
		return &APIError{Code: resp.Code, Message: resp.message()}
	}

	return nil
}

type apiResponse struct {
	Code    int             `json:"code"`
	Msg     string          `json:"msg"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (r apiResponse) message() string {
	if r.Message != "" {
		return r.Message
	}
	if r.Msg != "" {
		return r.Msg
	}
	return fmt.Sprintf("接口返回 code=%d", r.Code)
}

func postJSON(apiURL, token string, payload interface{}) (*apiResponse, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	body, err := doRequest(apiURL, token, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	var resp apiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, errors.Wrap(err, "接口返回的内容无法解析")
	}
	if resp.Code != 0 {
		return nil, &APIError{Code: resp.Code, Message: resp.message()}
	}

	return &resp, nil
}

func doRequest(apiURL, token, contentType string, body io.Reader) ([]byte, error) {
	request, err := http.NewRequest(http.MethodPost, apiURL, body)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("User-Agent", unityUserAgent)
	request.Header.Set("X-Unity-Version", unityVersion)
	request.Header.Set("Accept", "*/*")
	if token != "" {
		request.Header.Set("Authorization", token)
	}

	client := &http.Client{Timeout: requestTimeout}
	resp, err := client.Do(request)
	if err != nil {
		return nil, errors.Wrap(err, "网络请求失败")
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, &APIError{Code: resp.StatusCode, Message: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200))}
	}

	return respBody, nil
}

func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit]
}
