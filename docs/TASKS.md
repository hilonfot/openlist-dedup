# TASKS.md
> by chatgpt
> 项目：OpenList 媒体资源去重系统
>
> 当前状态：Phase 2 已完成
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

状态：⬜ 未开始

## 数据结构

### ScanTask

* [ ] Storage
* [ ] Path

### Scanner

* [ ] Worker Pool
* [ ] Task Queue
* [ ] Result Queue

---

## BFS扫描

### 任务

* [ ] 广度优先扫描
* [ ] 禁止递归
* [ ] 深层目录支持

---

## 统计信息

### 输出

* [ ] 当前Worker数量
* [ ] 已扫描目录数
* [ ] 已扫描文件数
* [ ] 当前速度
* [ ] ETA

---

## 性能

目标：

* [ ] 10万文件
* [ ] 100层目录
* [ ] 无栈溢出

---

## 测试

* [ ] Queue测试
* [ ] Worker测试
* [ ] 并发测试

---

## 验收

* [ ] 测试通过
* [ ] Git Commit

---

# Phase 4：SQLite索引层

状态：⬜ 未开始

## Schema

### media_files

* [ ] 创建表
* [ ] 创建索引

### scan_tasks

* [ ] 创建表
* [ ] 创建索引

### tmdb_cache

* [ ] 创建表
* [ ] 创建索引

---

## Repository

### 文件索引

* [ ] BatchInsert
* [ ] QueryByPath
* [ ] QueryByName
* [ ] QueryByStorage

---

### 扫描状态

* [ ] SaveTask
* [ ] UpdateTask
* [ ] LoadPendingTasks

---

## 批量写入

规则：

* [ ] 1000条提交
* [ ] 5秒强制Flush

---

## 恢复机制

* [ ] 崩溃恢复
* [ ] 重启恢复
* [ ] 未完成任务恢复

---

## 测试

* [ ] SQLite测试
* [ ] Batch测试
* [ ] Resume测试

---

## 验收

* [ ] 10万文件写入成功
* [ ] Git Commit

---

# Phase 5：媒体标准化

状态：⬜ 未开始

## 电影名称

### 清理内容

* [ ] 2160P
* [ ] 1080P
* [ ] 720P
* [ ] HDR
* [ ] DV
* [ ] BluRay
* [ ] WEB-DL
* [ ] WEBRip
* [ ] x265
* [ ] H265
* [ ] AAC

---

## 分隔符统一

* [ ] .
* [ ] -
* [ ] _

替换为空格

---

## 剧集识别

支持：

* [ ] S01E01
* [ ] S01E02
* [ ] EP01
* [ ] EP02
* [ ] 第01集
* [ ] 第1季第1集

---

## 输出格式

示例：

* [ ] 庆余年_S01E01
* [ ] TheLastOfUs_S01E01

---

## 测试

* [ ] 中文电影
* [ ] 英文电影
* [ ] 中文剧集
* [ ] 英文剧集

---

## 验收

* [ ] 准确率 > 90%
* [ ] Git Commit

---

# Phase 6：重复检测

状态：⬜ 未开始

## 第一层

* [ ] 标准化名称匹配

---

## 第二层

* [ ] 文件大小比较

规则：

* [ ] 误差 < 1%

---

## 第三层

* [ ] TMDB ID

---

## 去重策略

### 保留优先级

1. 本地
2. 天翼
3. 夸克

---

### 输出

* [ ] Keep
* [ ] Delete

---

## 统计

* [ ] 重复文件数
* [ ] 重复容量

---

## 测试

* [ ] 重复电影
* [ ] 重复剧集
* [ ] 跨存储重复

---

## 验收

* [ ] Git Commit

---

# Phase 7：TMDB集成

状态：⬜ 未开始

## Client

* [ ] SearchMovie
* [ ] SearchTV

---

## 缓存

* [ ] SQLite缓存
* [ ] TTL

---

## 多语言

* [ ] 中文
* [ ] 英文

---

## 匹配增强

* [ ] 年份辅助
* [ ] 季数辅助
* [ ] 模糊匹配

---

## 测试

* [ ] Movie测试
* [ ] TV测试
* [ ] Cache测试

---

## 验收

* [ ] Git Commit

---

# Phase 8：HTML报告

状态：⬜ 未开始

## 页面

### 重复电影

* [ ] 名称
* [ ] 存储位置
* [ ] 大小

### 重复电视剧

* [ ] 名称
* [ ] 集数
* [ ] 存储位置

---

### 删除建议

* [ ] Keep
* [ ] Delete
* [ ] 节省空间

---

### 存储统计

* [ ] 文件数
* [ ] 总容量
* [ ] 重复容量

---

## 图表

* [ ] 容量占比
* [ ] 存储占比
* [ ] 重复容量

---

## 输出

* [ ] report.html

---

## 验收

* [ ] 浏览器正常打开
* [ ] Git Commit

---

# Phase 9：自动清理

状态：⬜ 未开始

## Cleanup Plan

* [ ] cleanup_plan.json

---

## Dry Run

* [ ] 默认开启

---

## Real Delete

要求：

* [ ] --apply
* [ ] 二次确认

---

## OpenList删除

* [ ] Remove API

---

## 测试

* [ ] Dry Run测试
* [ ] Delete测试

---

## 验收

* [ ] Git Commit

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
