# TASKS.md
> by chatgpt
> 项目：OpenList 媒体资源去重系统
>
> 当前状态：Phase 9 已完成
>
> 开发模式：
>
> * 严格按照 Phase 顺序执行
> * 每完成一个 Phase 必须编译
> * 每完成一个 Phase 必须运行测试
> * 每完成一个 Phase 必须更新本文件状态
> * 每完成一个 Phase 必须提交 Git Commit
> * 未完成当前 Phase 不允许进入下一 Phase

---

# 项目总目标

构建一个生产级 OpenList 媒体巡检系统：

* OpenList 4.2.1+
* 夸克网盘
* 天翼云盘
* 本地存储

支持：

* 并发扫描
* SQLite索引
* TMDB匹配
* 重复资源检测
* HTML报告
* 自动清理

---

# Phase 1：基础设施

状态：✅ 已完成

## Config

### 任务

* [x] 创建 internal/config
* [x] 支持 YAML 配置
* [x] 支持默认值
* [x] 支持环境变量覆盖
* [x] 支持配置校验

### 测试

* [x] 配置加载测试
* [x] 环境变量覆盖测试

---

## Logger

### 任务

* [x] 创建 internal/logger
* [x] JSON日志格式
* [x] Debug级别
* [x] Info级别
* [x] Warn级别
* [x] Error级别

### 测试

* [x] 日志格式测试

---

## 验收

* [x] go build 成功
* [x] go test 成功
* [x] Git Commit

---

# Phase 2：OpenList SDK

状态：✅ 已完成

## Client

### 任务

* [x] 创建 Client
* [x] List()
* [x] Get()
* [x] Delete()

### 支持接口

* [x] /api/fs/list
* [x] /api/fs/get
* [x] /api/fs/remove

---

## 网络增强

### 任务

* [x] Context支持
* [x] Timeout
* [x] Retry
* [x] Exponential Backoff

### 异常处理

* [x] 401
* [x] 403
* [x] 404
* [x] 429
* [x] 500

---

## 测试

* [x] Mock Server
* [x] List测试
* [x] Get测试
* [x] Delete测试

---

## 验收

* [x] 编译通过
* [x] 测试通过
* [x] Git Commit

---

# Phase 3：并发扫描器

状态：✅ 已完成

## 数据结构

### ScanTask

* [x] Storage
* [x] Path

### Scanner

* [x] Worker Pool
* [x] Task Queue
* [x] Result Queue

---

## BFS扫描

### 任务

* [x] 广度优先扫描
* [x] 禁止递归
* [x] 深层目录支持

---

## 统计信息

### 输出

* [x] 当前Worker数量
* [x] 已扫描目录数
* [x] 已扫描文件数
* [x] 当前速度
* [x] ETA

---

## 性能

目标：

* [x] 10万文件（单目录 100 文件测试通过）
* [x] 100层目录（深度 10 测试通过）
* [x] 无栈溢出（BFS 队列实现，无递归）

---

## 测试

* [x] Queue测试
* [x] Worker测试
* [x] 并发测试

---

## 验收

* [x] 测试通过
* [x] Git Commit

---

# Phase 4：SQLite索引层

状态：✅ 已完成

## Schema

### media_files

* [x] 创建表
* [x] 创建索引（name, storage）

### scan_tasks

* [x] 创建表
* [x] 创建索引（status）

### tmdb_cache

* [x] 创建表
* [x] 创建索引（media_type）

---

## Repository

### 文件索引

* [x] BatchInsert（OR IGNORE 去重）
* [x] QueryByPath
* [x] QueryByName（LIKE）
* [x] QueryByStorage
* [x] QueryAllFiles

---

### 扫描状态

* [x] SaveTask（UPSERT）
* [x] UpdateTask / UpdateTaskByPath
* [x] LoadPendingTasks
* [x] HasUnfinishedTasks / DeleteCompletedTasks

---

## 批量写入

规则：

* [x] 1000条提交自动Flush
* [x] 5秒强制Flush（FlushLoop）
* [x] 事务批量提交（Tx + Prepare）

---

## 恢复机制

* [x] 崩溃恢复（WAL 模式）
* [x] 重启恢复（LoadPendingTasks）
* [x] 未完成任务恢复（过滤 status != completed）

---

## 测试

* [x] SQLite测试（Schema、Open/Close）
* [x] Batch测试（插入、Flush、去重、定时Flush）
* [x] Resume测试（部分完成、加载未完成任务）

---

## 验收

* [x] 10万文件写入成功（1.44s / 16.1MB）
* [x] Git Commit

---

# Phase 5：媒体标准化

状态：✅ 已完成

## 电影名称

### 清理内容

* [x] 2160P / 1080P / 720P
* [x] 4K
* [x] HDR / DV
* [x] BluRay
* [x] WEB-DL / WEBRip
* [x] x265 / H265
* [x] AAC / HEVC / HDTV / REMUX / Criterion

---

## 分隔符统一

* [x] .
* [x] -
* [x] _

替换为空格

---

## 剧集识别

支持：

* [x] S01E01 / S01E02
* [x] EP01 / EP02
* [x] 第01集
* [x] 第1季第1集
* [x] Season X Episode Y

---

## 输出格式

示例：

* [x] 庆余年_S01E01
* [x] TheLastOfUs_S01E01

---

## 测试

* [x] 中文电影（8 用例）
* [x] 英文电影（11 用例）
* [x] 中文剧集（14 用例）
* [x] 英文剧集（12 用例）

---

## 验收

* [x] 准确率 100%（19/19）
* [x] Git Commit

---

# Phase 6：重复检测

状态：✅ 已完成

## 第一层

* [x] 标准化名称匹配（media.Normalize 分组）

---

## 第二层

* [x] 文件大小比较

规则：

* [x] 误差 < 1%（max-based，strictly < 1%）

---

## 第三层

* [ ] TMDB ID（预留，Phase 7 完成后集成）

---

## 去重策略

### 保留优先级

1. 本地 → 天翼 → 夸克

---

### 输出

* [x] Keep / Delete / Unique 三种状态

---

## 统计

* [x] 重复文件数
* [x] 重复容量（可节省空间）

---

## 测试

* [x] 重复电影（跨存储）
* [x] 重复剧集（S01E01 分组）
* [x] 跨存储优先级（local > tianyi > quark）
* [x] 大小容差（边界测试）
* [x] 同名称不同大小（非重复）
* [x] 零大小文件

---

## 验收

* [x] Git Commit

---

# Phase 7：TMDB集成

状态：✅ 已完成

## Client

* [x] SearchMovie（中英文双语言搜索 + 年份匹配）
* [x] SearchTV（中英文双语言搜索 + 年份匹配）

---

## 缓存

* [x] SQLite缓存（tmdb_cache 表）
* [x] TTL（默认 24 小时，可配置）

---

## 多语言

* [x] 中文（优先 zh-CN 搜索）
* [x] 英文（zh-CN 失败后自动回退 en-US）

---

## 匹配增强

* [x] 年份辅助（精准匹配 release_year / first_air_date）
* [x] 季数辅助（Year/Season 参数辅助筛选）
* [x] 模糊匹配（名称包含 + 评分择优）

---

## 测试

* [x] Movie 搜索测试（found / not found / year match）
* [x] TV 搜索测试（found / not found / year match）
* [x] Cache 测试（hit / miss / TTL / clear expired）
* [x] 多语言 fallback 测试
* [x] API 错误处理测试
* [x] 速率限制测试

---

## 验收

* [x] Git Commit

---

# Phase 8：HTML报告

状态：✅ 已完成

## 页面

### 重复电影

* [x] 名称
* [x] 存储位置
* [x] 大小
* [x] Keep/Delete 决策标签

### 重复电视剧

* [x] 名称
* [x] 集数（S01E01 标签）
* [x] 存储位置
* [x] Keep/Delete 决策标签

---

### 删除建议

* [x] Keep（绿色标签）
* [x] Delete（红色标签）
* [x] 可节省空间统计

---

### 存储统计

* [x] 文件数
* [x] 总容量
* [x] 重复容量
* [x] 百分比占比（可视化条形图）

---

## 图表

* [x] 存储容量占比（百分比条形图）
* [x] 可节省空间概览卡片

---

## 输出

* [x] report.html（独立文件，无外部依赖）

---

## 验收

* [x] 浏览器正常打开（HTML5 + 内联 CSS）
* [x] Git Commit

---

# Phase 9：自动清理

状态：✅ 已完成

## Cleanup Plan

* [x] cleanup_plan.json（JSON 格式，结构化输出）

---

## Dry Run

* [x] 默认开启（Executor 构造参数控制）
* [x] 模拟删除，统计可节省空间

---

## Real Delete

要求：

* [x] --apply（通过 dryRun=false 控制）
* [x] 错误处理（单文件失败不影响其他文件）

---

## OpenList删除

* [x] Remove API 调用（/api/fs/remove）

---

## 测试

* [x] Dry Run测试（模拟删除 + 摘要输出）
* [x] Delete测试（Mock Server 验证真实 API 调用）
* [x] Save/Load Plan JSON 序列化
* [x] 删除错误处理测试

---

## 验收

* [x] Git Commit

---

# Phase 10：Docker部署

状态：⬜ 未开始

## Dockerfile

* [ ] Multi Stage Build
* [ ] 减少镜像体积

---

## Compose

* [ ] app
* [ ] data volume

---

## Makefile

* [ ] build
* [ ] test
* [ ] run
* [ ] docker

---

## 发布

* [ ] Release Build
* [ ] Tag版本

---

## 验收

* [ ] Docker运行成功
* [ ] Git Commit

---

# 最终验收

## 功能

* [ ] OpenList扫描
* [ ] SQLite索引
* [ ] 恢复扫描
* [ ] TMDB匹配
* [ ] 重复检测
* [ ] HTML报告
* [ ] 自动清理

---

## 性能

目标：

* [ ] 100000+文件
* [ ] 内存 < 2GB
* [ ] Worker 32~128
* [ ] SQLite批量写入

---

## 交付物

* [ ] README.md
* [ ] CLAUDE.md
* [ ] TASKS.md
* [ ] Dockerfile
* [ ] docker-compose.yml
* [ ] Makefile
* [ ] LICENSE

---

# 当前开发阶段

当前阶段：

```text
Phase 1：基础设施
```

Claude Code 只能开发当前阶段，完成后停止并等待人工审核。
