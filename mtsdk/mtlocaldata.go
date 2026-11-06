// Package mtsdk reads the login credential that the Mica PC SDK persists for
// 少女前线2：追放 (GF2 Exilium) and turns it into a usable access token.
//
// 客户端把登录状态写在这里:
//
//	<游戏>/GF2_Exilium_Data/StreamingAssets/MT_PC_SDK_Data/MTLocalData
//
// 文件是 AES-128-CBC(PKCS#7) 加密的 JSON 字典，密钥与 IV 是编译进
// GameAssembly.dll 的两个字符串字面量（SDKLocalDataUtil..cctor）。
//
// 读取流程：
//
//	MTLocalData --解密--> access_token --account/auto_login--> 当前可用的 access_token
//
// 整个过程不抓包、不装证书、不修改任何游戏文件。
package mtsdk

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"os"

	"github.com/pkg/errors"
)

const (
	// AESKey / AESIV 对应 GameAssembly.dll 中 SDKLocalDataUtil..cctor 里的
	// UTF-8 字符串字面量。改动它们等于改动协议，谨慎对待。
	AESKey = "omn2vrk4dbvqrxzb"
	AESIV  = "inbo22dyurbnng53"

	// EnvLocalDataPath 可覆盖自动探测到的 MTLocalData 路径（调试用）。
	EnvLocalDataPath = "GF2_MTLOCALDATA"

	blockSize = aes.BlockSize
)

// Credentials 是 MTLocalData 中与抽卡记录相关的字段。
type Credentials struct {
	AccessToken     string
	UcAccountToken  string
	DeviceNum       string
	GameUid         string
	LastLoginTimeMs int64
	Raw             map[string]string
}

// userBean 对应 key_user_now 里保存的 UserBean 子集。
type userBean struct {
	AccessToken    string `json:"access_token"`
	UcAccountToken string `json:"uc_account_token"`
	Uid            int64  `json:"uid"`
	LastLoginTime  int64  `json:"lastLoginTime"`
	PhoneNum       string `json:"phoneNum"`
}

// Decrypt 解密 MTLocalData，返回其中的字符串字典。
// 若文件本身是明文 JSON（手工构造的测试数据），也直接接受。
func Decrypt(data []byte) (map[string]string, error) {
	if len(data) == 0 {
		return nil, errors.New("MTLocalData 是空文件")
	}

	plain := data
	if !isJSONObject(data) {
		if len(data)%blockSize != 0 {
			return nil, errors.Errorf("MTLocalData 长度 %d 不是 %d 的整数倍,可能不是加密文件", len(data), blockSize)
		}

		block, err := aes.NewCipher([]byte(AESKey))
		if err != nil {
			return nil, errors.WithStack(err)
		}

		plain = make([]byte, len(data))
		cipher.NewCBCDecrypter(block, []byte(AESIV)).CryptBlocks(plain, data)

		plain, err = unpadPKCS7(plain)
		if err != nil {
			return nil, err
		}
	}

	var values map[string]string
	if err := json.Unmarshal(plain, &values); err != nil {
		return nil, errors.Wrap(err, "MTLocalData 解密后不是合法的 JSON 字典")
	}

	return values, nil
}

// LoadCredentials 读取并解析 MTLocalData。
func LoadCredentials(path string) (*Credentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	values, err := Decrypt(data)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	creds := &Credentials{
		DeviceNum: values["key_mts_owner_device_number"],
		GameUid:   values["key_mt_cs_game_uid"],
		Raw:       values,
	}

	if userJSON := values["key_user_now"]; userJSON != "" {
		var user userBean
		if err := json.Unmarshal([]byte(userJSON), &user); err != nil {
			return nil, errors.Wrap(err, "key_user_now 不是合法的 JSON")
		}
		creds.AccessToken = user.AccessToken
		creds.UcAccountToken = user.UcAccountToken
		creds.LastLoginTimeMs = user.LastLoginTime
	}

	if creds.AccessToken == "" {
		creds.AccessToken = values["key_token_now"]
	}

	if creds.AccessToken == "" {
		return nil, errors.New("MTLocalData 中没有 access_token,请先启动游戏并完成一次登录")
	}

	return creds, nil
}

func isJSONObject(data []byte) bool {
	for _, b := range data {
		switch b {
		case ' ', '\t', '\r', '\n':
			continue
		case '{':
			return true
		default:
			return false
		}
	}
	return false
}

func unpadPKCS7(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("MTLocalData 解密结果为空")
	}

	padding := int(data[len(data)-1])
	if padding < 1 || padding > blockSize || padding > len(data) {
		return nil, errors.New("MTLocalData 填充无效,密钥或文件可能不匹配")
	}
	for _, b := range data[len(data)-padding:] {
		if int(b) != padding {
			return nil, errors.New("MTLocalData 填充无效,密钥或文件可能不匹配")
		}
	}

	return data[:len(data)-padding], nil
}
