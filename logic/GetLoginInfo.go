package logic

import (
	"path/filepath"
	"strconv"
	"sync"

	"gf2gacha/logger"
	"gf2gacha/model"
	"gf2gacha/mtsdk"
	"gf2gacha/util"

	"github.com/pkg/errors"
)

var (
	cachedLoginInfo model.LogInfo
	cachedLoginOK   bool
	cachedLoginMu   sync.Mutex
)

// GetLoginInfo 返回拉取抽卡记录所需的登录信息。
//
// 优先读取游戏客户端本地保存的凭证(MTLocalData)，这样不依赖 Player.log，
// 也不需要抓包；本地凭证不可用时再退回日志。
func GetLoginInfo() (model.LogInfo, error) {
	localInfo, localErr := ReadLocalLoginInfo()
	if localErr == nil {
		return localInfo, nil
	}

	logInfo, logErr := util.GetLogInfo()
	if logErr == nil && logInfo.AccessToken != "" && logInfo.Uid != "" && logInfo.GachaUrl != "" {
		return logInfo, nil
	}

	return model.LogInfo{}, errors.Errorf(
		"无法获取登录信息: 读取本地凭证失败(%v), 读取游戏日志失败(%v)", localErr, logErr)
}

// ReadLocalLoginInfo 读取 MTLocalData 里的登录凭证，必要时用 account/auto_login
// 换取当前有效的 access_token。不会修改任何游戏文件。
func ReadLocalLoginInfo() (model.LogInfo, error) {
	gameDataDir, err := util.GetGameDataDir()
	if err != nil {
		return model.LogInfo{}, errors.WithStack(err)
	}

	dataPath, err := mtsdk.FindLocalDataPath(gameDataDir)
	if err != nil {
		return model.LogInfo{}, errors.WithStack(err)
	}

	creds, err := mtsdk.LoadCredentials(dataPath)
	if err != nil {
		return model.LogInfo{}, errors.Wrapf(err, "解析 %s 失败", dataPath)
	}

	gachaURL, channel := resolveGachaURL()
	token := creds.AccessToken

	if err = mtsdk.ProbeToken(token, channel); err != nil {
		if !mtsdk.IsTokenError(err) {
			return model.LogInfo{}, errors.Wrap(err, "校验本地令牌失败")
		}

		logger.Logger.Infof("本地令牌(%s)已不是当前会话令牌: %v, 尝试 account/auto_login", maskToken(token), err)
		login, loginErr := mtsdk.AutoLogin(token, creds.DeviceNum, channel, mtsdk.ReyunChannelID(gameDataDir))
		if loginErr != nil {
			if mtsdk.IsTokenError(loginErr) {
				return model.LogInfo{}, errors.Wrap(loginErr,
					"本地保存的令牌已过期,请启动一次游戏重新登录后再试")
			}
			return model.LogInfo{}, errors.Wrap(loginErr, "刷新令牌失败")
		}
		token = login.AccessToken
	}

	uid := creds.GameUid
	if uid == "" {
		userInfo, bbsErr := GetUserInfoFromBBS(token)
		if bbsErr != nil {
			return model.LogInfo{}, errors.Wrap(bbsErr, "未能获取账号 UID")
		}
		uid = strconv.FormatInt(userInfo.User.GameUid, 10)
	}

	info := model.LogInfo{
		TablePath:   filepath.Join(gameDataDir, "Table"),
		AccessToken: token,
		Uid:         uid,
		GachaUrl:    gachaURL,
	}
	cacheLoginInfo(info)

	logger.Logger.Infof("已从本地凭证读取登录信息 UID:%s", info.Uid)

	return info, nil
}

// resolveGachaURL 优先沿用日志里记录过的抽卡链接（渠道号以它为准），
// 没有日志时按官服渠道号拼一个。
func resolveGachaURL() (string, int64) {
	if logInfo, err := util.GetLogInfo(); err == nil && logInfo.GachaUrl != "" {
		if channel, ok := mtsdk.ParseChannel(logInfo.GachaUrl); ok {
			return logInfo.GachaUrl, channel
		}
		return logInfo.GachaUrl, mtsdk.DefaultGameChannelID
	}

	return mtsdk.GachaURL(mtsdk.DefaultGameChannelID), mtsdk.DefaultGameChannelID
}

// GetCachedLoginInfo 返回最近一次从本地凭证读到的登录信息。
// 供「关于」弹窗展示，避免展示时触发网络请求。
func GetCachedLoginInfo() (model.LogInfo, bool) {
	cachedLoginMu.Lock()
	defer cachedLoginMu.Unlock()

	return cachedLoginInfo, cachedLoginOK
}

func cacheLoginInfo(info model.LogInfo) {
	cachedLoginMu.Lock()
	defer cachedLoginMu.Unlock()

	cachedLoginInfo = info
	cachedLoginOK = true
}

func maskToken(token string) string {
	if len(token) <= 28 {
		return token
	}
	return token[:24] + "..." + token[len(token)-8:]
}
