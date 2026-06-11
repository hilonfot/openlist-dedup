# OpenList 影视媒体识别与聚合规范

Version: v1.0

---

# 目标

实现类似 Jellyfin / Emby / Plex 的影视媒体识别系统。

支持：

* Movie（电影）
* TV（电视剧）

注意：

```text
动漫
美剧
日剧
韩剧
纪录片剧集
综艺
```

统一归类为：

```go
MediaTV
```

最终系统只保留：

```go
const (
    MediaMovie = "movie"
    MediaTV    = "tv"
)
```

---

# 总体流程

```text
扫描目录
    ↓
发现视频文件
    ↓
识别 Movie / TV
    ↓
清洗文件名
    ↓
提取 Title
    ↓
提取 Year
    ↓
提取 Season/Episode
    ↓
TV 聚合
    ↓
TMDB 搜索
    ↓
生成媒体库
```

---

# 第一部分：媒体类型识别

采用评分机制。

## 数据结构

```go
type MediaGuess struct {
    MovieScore int
    TVScore    int
}
```

最终：

```go
if TVScore > MovieScore {
    return MediaTV
}

return MediaMovie
```

---

# TV 识别规则

## Rule 1：SxxExx

匹配：

```regex
(?i)S\d{1,2}E\d{1,3}
```

示例：

```text
Breaking.Bad.S01E01.mkv
The.Last.of.Us.S01E02.mkv
```

加分：

```go
TV += 100
```

---

## Rule 2：Season目录

目录名：

```text
Season 1
Season 01
S01
S02
第一季
第1季
```

加分：

```go
TV += 80
```

---

## Rule 3：第XX集

匹配：

```regex
第\d+集
```

加分：

```go
TV += 50
```

---

## Rule 4：E01模式

匹配：

```regex
(?i)\bE\d{1,3}\b
```

加分：

```go
TV += 50
```

---

## Rule 5：动漫连续编号

示例：

```text
葬送的芙莉莲 - 01.mkv
葬送的芙莉莲 - 02.mkv
葬送的芙莉莲 - 03.mkv
```

匹配：

```regex
[- ]\d{2,3}
```

条件：

```go
连续编号文件 >= 2
```

加分：

```go
TV += 40
```

---

## Rule 6：多视频文件

条件：

```go
videoFiles >= 3
```

加分：

```go
TV += 30
```

---

# Movie识别规则

## Rule 1：单视频文件

```go
videoFiles == 1
```

加分：

```go
Movie += 50
```

---

## Rule 2：包含年份

匹配：

```regex
(19|20)\d{2}
```

加分：

```go
Movie += 20
```

---

## Rule 3：时长

需要 ffprobe。

规则：

```go
duration > 80min
```

加分：

```go
Movie += 40
```

---

# 第二部分：文件名清洗

目标：

```text
The.Last.of.Us.S01E01.2160p.WEB-DL.DDP5.1.H265-FLUX.mkv
```

变成：

```text
The Last of Us
```

---

# 保留内容

必须保留：

```text
Title
Year
Season
Episode
```

---

# 删除质量标签

## Resolution

```text
4320P
2880P
2160P
1440P
1080P
1080I
720P
576P
480P

8K
4K
UHD
FHD
QHD
HD
SD
```

---

## HDR

```text
HDR
HDR10
HDR10+
DV
DoVi
DolbyVision
HLG
SDR

BT2020
REC2020
WCG
```

---

## Video Codec

```text
x264
x265

H264
H265

HEVC
AVC
AV1
VP9

XVID
DivX
```

---

## Audio Codec

```text
AAC
AAC2.0
AAC5.1

DD
DDP
DD+

AC3
EAC3

DTS
DTSHD
DTS-HD

TrueHD
Atmos

FLAC
PCM
LPCM
```

---

## Source

```text
BluRay
BDRip
BDREMUX
REMUX

BRRip

WEB-DL
WEBDL
WEBRip

HDTV
DVDRip
DVD

CAM
TS
TC
R5
```

---

## Streaming Source

```text
Netflix
NF

Disney+
DSNP

AMZN
Amazon

ATVP
AppleTV+

HMAX
MAX

HULU
```

---

## Subtitle Tags

```text
中字
双字
双语

简体
繁体
简繁

CHS
CHT

ENG
JPN
KOR

内嵌
内封

外挂字幕
特效字幕
```

---

## Chinese Release Tags

```text
国语
粤语

国粤双语
国英双语

国配
港配
台配

原盘
DIY
菜单修改
```

---

## Scene Tags

```text
PROPER
REPACK
RERIP

INTERNAL

EXTENDED
THEATRICAL

UNCUT
UNRATED

LIMITED
HYBRID
```

---

# 第三部分：发布组识别

不要放入质量标签。

单独处理。

```go
var releaseGroups = []string{
    "CMRG",
    "EVO",
    "SPARKS",
    "FGT",
    "NTB",
    "FLUX",
    "CHD",
    "WiKi",
    "MTeam",
    "PTer",
    "OurBits",
    "FRDS",
    "BeiTai",
}
```

匹配：

```text
Movie.2024.2160p.WEB-DL-FLUX
Movie.2024.1080p.BluRay-SPARKS
```

正则：

```regex
-(\w+)$
```

删除尾部发布组。

---

# 第四部分：Title提取

处理顺序：

```text
删除扩展名
    ↓
统一分隔符
    ↓
提取年份
    ↓
提取SxxExx
    ↓
删除质量标签
    ↓
删除发布组
    ↓
规范空格
    ↓
得到Title
```

---

# 第五部分：TV聚合

目标：

电视剧只展示一张海报。

---

## 示例

目录：

```text
绝命毒师
├── S01E01.mkv
├── S01E02.mkv
├── S01E03.mkv
├── S02E01.mkv
```

最终：

```json
{
  "type":"tv",
  "title":"绝命毒师",
  "season_count":2,
  "episode_count":4
}
```

海报墙只显示：

```text
绝命毒师
```

点击后进入：

```text
Season 1
 ├── Episode 1
 ├── Episode 2
 └── Episode 3

Season 2
 └── Episode 1
```

---

# 聚合Key

```go
key := normalize(title) + "-" + year
```

示例：

```text
The Last of Us (2023)
```

生成：

```text
the-last-of-us-2023
```

所有剧集归属同一个Series。

---

# 数据结构

```go
type Series struct {
    TMDBID       int64

    Title        string
    OriginalName string

    Year         int

    Seasons      map[int]*Season

    EpisodeCount int

    Path         string
}

type Season struct {
    Number   int
    Episodes []*Episode
}

type Episode struct {
    Season     int
    Episode    int

    Name       string
    FilePath   string
}
```

---

# 第六部分：TMDB匹配

搜索：

```text
Title
Title + Year
```

优先匹配：

```text
TV
Movie
```

返回：

```json
{
  "tmdb_id":12345,
  "type":"tv",
  "title":"The Last of Us",
  "year":2023
}
```

---

# 第七部分：最终媒体库结构

```go
type LibraryItem struct {
    ID          string

    Type        string

    Title       string
    Original    string

    Year        int

    TMDBID      int64

    Poster      string
    Backdrop    string

    Seasons     int
    Episodes    int

    Path        string
}
```

---

# 最终目标

海报墙展示：

```text
流浪地球2
绝命毒师
葬送的芙莉莲
权力的游戏
```

而不是：

```text
S01E01
S01E02
S01E03
```

电视剧必须聚合为一个Series实体。

---

# 目标准确率

| 类型    | 目标   |
| ----- | ---- |
| Movie | >98% |
| TV    | >95% |
| Anime | >90% |

