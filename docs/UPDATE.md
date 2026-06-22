我分析了这份 `openlist-dedup-main` 项目代码（Go + OpenList API + SQLite + TMDB + 媒体识别/去重）。整体看，这个项目比普通“扫描删除重复文件脚本”成熟很多，已经有：

* OpenList API Client
* 并发扫描 Worker Pool
* 媒体名称归一化
* 剧集识别
* 重复检测
* TMDB 元数据缓存
* 删除清理流程
* 报告输出

整体方向是对的。

不过如果目标是你之前说的 **“飞牛 NAS + OpenList + 影视库整理 + Jellyfin 类识别”生产化方案**，目前还有不少需要优化。

---

# 总体架构评价

当前：

```
OpenList
   |
   |
Scanner
   |
   |
Media Normalize
   |
   |
Duplicate Engine
   |
   |
TMDB
   |
   |
Report / Cleanup
```

评分：

| 模块    | 评价     |
| ----- | ------ |
| 架构    | 8/10   |
| Go实现  | 8.5/10 |
| 扫描性能  | 8/10   |
| 影视识别  | 7/10   |
| 去重可靠性 | 7/10   |
| 生产安全性 | 6.5/10 |

最大风险：

> 删除逻辑和影视识别准确率。

因为 NAS 影视库最怕误删。

---

# 1. 最大 Bug：扫描任务可能死锁

位置：

`scanner/scanner.go`

这里：

```go
s.pending.Add(len(seeds))

for _, task := range seeds {
    s.taskCh <- task
}
```

问题：

如果：

```yaml
queue_size: 10000
```

但是：

seed > 10000

例如：

一次扫描：

20000 个目录。

这里：

```go
s.taskCh <- task
```

会阻塞。

因为 worker 还没开始消费。

建议：

改成异步 enqueue：

```go
for _, task := range seeds {

    go s.enqueue(ctx, task)

}
```

或者：

启动 worker 后再投递。

---

# 2. Context取消存在资源泄漏风险

这里：

```go
case <-ctx.Done():

    return
```

worker 直接退出。

但是：

pending 里面可能还有任务。

虽然你有：

```go
s.pending.Done()
```

但是只在 enqueue 失败时。

扫描过程中：

```
task A
 |
发现100个子目录
 |
ctx cancel
```

可能：

pending 永远无法归零。

建议：

增加：

```go
defer func(){

for range remainingTasks {

 pending.Done()

}

}()
```

---

# 3. OpenList Token 没有自动刷新

现在：

```go
c.token = resp.Token
```

问题：

OpenList token 通常有有效期。

长期运行：

几天后：

```
401
 |
扫描失败
```

建议：

增加：

```go
func (c *Client) ensureToken()
```

逻辑：

```
request

 ↓

401

 ↓

login

 ↓

retry
```

---

# 4. 配置文件有安全问题

现在：

```yaml
password: "admin"
```

生产风险很大。

建议：

支持：

环境变量。

例如：

```yaml
password: ${OPENLIST_PASSWORD}
```

Docker:

```yaml
environment:

 OPENLIST_PASSWORD=xxxx
```

---

# 5. 媒体识别存在误判风险

当前 normalize 做得不错。

但是：

比如：

```
The.Last.of.Us.S01E01.2160p.WEB-DL.mkv
```

处理：

很好。

但是：

电影：

```
1917.2019.1080p.mkv
```

可能：

识别：

```
title=1917
year=2019
```

没问题。

但是：

```
2012.mkv
```

会：

识别成：

年份。

需要增加：

年份规则：

如果：

```
filename == 4位数字
```

不要作为年份。

---

# 6. 剧集识别缺少目录上下文

现在主要：

文件名。

但是 NAS：

大量：

```
Breaking Bad/

 Season 01/

 S01E01.mkv

```

文件名：

```
S01E01.mkv
```

无法知道：

电视剧名。

建议：

Scanner 输出：

增加：

```go
type FileEntry struct {

Name

Path

ParentFolders []string

}
```

识别：

优先：

```
文件名

↓

当前目录

↓

上级目录
```

这会大幅提升电视剧识别。

---

# 7. Duplicate 算法有误删风险

当前：

核心：

```
NormalizedName
+
Episode
+
Year
+
Size
```

问题：

两个不同版本：

```
流浪地球2 2023

4K REMUX 90GB

1080P WEB 8GB
```

normalize 后：

都是：

```
流浪地球2
```

然后：

进入重复。

实际：

不是重复。

建议增加：

质量指纹。

例如：

```go
type MediaFingerprint struct {


Resolution

Codec

HDR

Audio

Size


}
```

比较：

```
同电影

不同质量

保留多个版本
```

---

# 8. 去重策略需要支持“保留策略”

现在：

```go
storagePriority
```

例如：

```go
local
tianyi
quark
```

但是影视场景：

应该：

不是存储优先。

应该：

质量优先。

例如：

A:

```
本地
1080p
```

B:

```
夸克
4K REMUX
```

应该保留 B。

建议：

评分：

```
+4K
+HDR
+REMUX
+码率

-低码率
-压缩版

```

---

# 9. TMDB 依赖过强

现在：

流程：

```
normalize

↓

TMDB

↓

判断
```

问题：

国内影视：

大量：

* 国产剧
* 网盘资源名
* 中文字幕组名

TMDB 搜索失败。

建议：

增加 fallback：

```
TMDB

↓

豆瓣

↓

本地规则

↓

人工映射
```

增加：

```yaml
mapping.yaml
```

例如：

```yaml
斗破苍穹:
 tmdb:12345
```

---

# 10. SQLite 并发问题

scanner worker:

32

同时写：

TMDB cache

SQLite 默认：

容易：

```
database locked
```

建议：

打开：

WAL:

```sql
PRAGMA journal_mode=WAL;

PRAGMA busy_timeout=5000;
```

---

# 11. 缺少 dry-run 模式

删除系统必须有。

现在：

危险。

建议：

增加：

```bash
openlist-dedup --dry-run
```

输出：

```
将删除:

xxx.mkv

原因:

重复

保留:

xxx.4k.mkv

```

确认后：

再执行。

---

# 12. 缺少回滚机制

删除：

应该：

移动到：

```
.trash
```

而不是：

delete。

例如：

```
/movie/a.mkv

↓

/.openlist-trash/movie/a.mkv
```

保留7天。

---

# 13. 建议增加 Web UI

现在：

CLI。

但是这个项目非常适合：

```
Gin
+
Vue
```

功能：

* 扫描状态
* 重复列表
* 海报墙
* 删除确认

类似：

```
TinyMediaManager
+
Jellyfin管理
```

---

# 推荐升级路线

## V1.1（必须）

* 修复 scanner 阻塞
* token 自动刷新
* SQLite WAL
* dry-run
* 删除回收站

## V1.2（影视增强）

增加：

```
MediaFingerprint

{
 title
 year
 season
 episode
 resolution
 codec
 hdr
 size
}
```

## V2.0

变成完整 NAS 影视管理系统：

```
                 Web UI

                    |

              Media Service

                    |

     +--------------+-------------+

 Scanner        Recognizer       Cleaner


                    |

                  TMDB


                    |

             Jellyfin/FNOS
```

---

结论：

这个项目目前已经不是“重复文件清理工具”，而是一个**影视资源治理引擎雏形**。

目前最大的三个问题：

1. **删除安全性不足**
2. **电视剧识别依赖文件名，缺目录语义**
3. **重复判断没有区分影片质量版本**

如果按你之前规划的“飞牛影视作为媒体中心”，下一步我建议重点改 **Media Intelligence Layer（影视智能识别层）**，价值最大。
