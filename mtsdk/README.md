# mtsdk — 本地登录凭证

这个包只做一件事：把 PC 客户端已经存在本地的登录凭证，换成可以读取抽卡记录的 `access_token`。
不抓包、不装证书、不修改游戏文件。

## 1. 凭证文件

```
<游戏目录>\GF2_Exilium_Data\StreamingAssets\MT_PC_SDK_Data\MTLocalData
```

- AES-128-CBC + PKCS#7
- 解密后是紧凑 JSON 字典（键和值都是字符串）
- 路径由 `util.GetGameDataDir()` 推导：`LocalCache/Data` 往上两级是 `GF2_Exilium_Data`
- 可用环境变量 `GF2_MTLOCALDATA` 覆盖（调试用）

### 密钥来源

`GameAssembly.dll` → `MTSDKBaseApi.Repository.LocalStorageTools.SDKLocalDataUtil..cctor`
（RVA `0x366C450`）用 `Encoding.UTF8.GetBytes(<字符串字面量>)` 初始化：

| 字段 | metadata usage 地址 | 字面量 |
| --- | --- | --- |
| `AES_KEY` | `0x496FFF8` | `omn2vrk4dbvqrxzb` |
| `AES_IV` | `0x49E8610` | `inbo22dyurbnng53` |

### 关键字段

| 键 | 说明 |
| --- | --- |
| `key_mts_owner_device_number` | `device_num`，`account/auto_login` 必填 |
| `key_user_now` | JSON 字符串，含 `access_token` / `uc_account_token` / `lastLoginTime` |
| `key_token_now` | `access_token` 的冗余副本 |
| `key_mt_cs_game_uid` | 社区 `game_uid`，与 `member/info` 返回的 `game_uid` 一致 |
| `key_mt_cs_token` | 客服体系令牌，**不是**抽卡令牌 |

## 2. 换令牌

```http
POST https://gf2-zoneinfo.sunborngame.com/account/auto_login
Authorization: <MTLocalData 里的 access_token>
Content-Type: application/json

{"game_channel_id":5,"device_num":"<device_num>","mac":"","os":3,"language":"cn","reyun_channel_id":"gwpc"}
```

- `reyun_channel_id` 来自 `LocalCache/Data/cl.bin`（官服为 `gwpc`）
- 本地令牌**不需要是当前会话令牌**：抽卡接口返回 `token err` 时，`auto_login` 依然接受它
- 返回 `data.access_token` 即为当前可用令牌
- 令牌有效期约 7 天；本地令牌和刷新结果都过期后，需要启动一次游戏重新登录

## 3. 读取抽卡记录

```http
POST https://gf2-gacha-record.sunborngame.com/list?game_channel_id=5&type_id=3
Authorization: <access_token>
Content-Type: application/x-www-form-urlencoded

type_id=<池子>&limit=20&next=<游标>
```

query 里的 `type_id` 固定为 3，真正的池子 id 在表单里。

## 4. 行为约定

- `ProbeToken` 发一次最小请求判断令牌是否可用，避免每次都触发 `auto_login`
- `IsTokenError` 同时识别抽卡接口的 `token err` 和账号接口的 `10004 访问令牌无效`
- 全程只读；`WriteBack` 之类会改动游戏文件的能力刻意没有实现
