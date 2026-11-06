package mtsdk

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// FindLocalDataPath 定位 MTLocalData。
//
// gameDataDir 是 util.GetGameDataDir() 的返回值，即
// <游戏>/GF2_Exilium_Data/LocalCache/Data。
// MTLocalData 则在 <游戏>/GF2_Exilium_Data/StreamingAssets/MT_PC_SDK_Data/ 下。
func FindLocalDataPath(gameDataDir string) (string, error) {
	if path := os.Getenv(EnvLocalDataPath); path != "" {
		if stat, err := os.Stat(path); err == nil && !stat.IsDir() {
			return filepath.Clean(path), nil
		}
		return "", errors.Errorf("环境变量 %s 指向的文件不存在: %s", EnvLocalDataPath, path)
	}

	var candidates []string
	if gameDataDir != "" {
		candidates = append(candidates,
			filepath.Join(gameDataDir, "..", "..", "StreamingAssets", "MT_PC_SDK_Data", "MTLocalData"))
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "MTLocalData"))
	}

	for _, candidate := range candidates {
		if stat, err := os.Stat(candidate); err == nil && !stat.IsDir() {
			return filepath.Clean(candidate), nil
		}
	}

	return "", errors.Errorf(
		"未找到 MTLocalData,请确认游戏已安装并至少登录过一次(游戏目录: %s)", gameDataDir)
}

// ReyunChannelID 读取 PC 客户端写入的热云渠道号(cl.bin)，官服为 gwpc。
func ReyunChannelID(gameDataDir string) string {
	if gameDataDir == "" {
		return DefaultReyunChannelID
	}

	data, err := os.ReadFile(filepath.Join(gameDataDir, "cl.bin"))
	if err != nil {
		return DefaultReyunChannelID
	}

	channel := strings.TrimSpace(string(data))
	if channel == "" {
		return DefaultReyunChannelID
	}

	return channel
}
