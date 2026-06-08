# [CLAUDE.md](http://CLAUDE.md)

>  by chatgpt

# 项目名称

OpenList 媒体资源去重系统

---

# 项目目标

开发一个生产级媒体巡检工具，用于扫描 OpenList 中的夸克网盘、天翼云盘和本地存储资源，建立统一媒体索引，并自动识别重复电影和电视剧资源。

支持：

- OpenList 4.2.1+
- 夸克网盘
- 天翼云盘
- 本地存储
- SQLite 索引
- TMDB 元数据匹配
- HTML 报告
- Docker 部署
- 自动删除重复资源（可选）

---

# 技术栈

语言：

```text
Go 1.24+
```

数据库：

```text
SQLite
```

部署：

```text
Docker
Docker Compose
```

要求：

- 优先使用标准库
- 尽量减少第三方依赖
- 所有接口支持 Context
- 使用结构化日志
- 所有模块可单元测试

---

# 项目结构

```text
cmd/

internal/
    config/
    logger/
    openlist/
    scanner/
    repository/
    media/
    duplicate/
    tmdb/
    report/

configs/
data/
```

规则：

- 禁止循环依赖
- 业务逻辑只能放在 internal
- cmd 目录只负责启动

---

# OpenList SDK

必须实现：

```go
List(path string)
Get(path string)
Delete(path string)
```

接口：

```text
POST /api/fs/list
POST /api/fs/get
POST /api/fs/remove
```

要求：

- 自动重试
- 指数退避
- 请求超时
- 错误分类

---

# Scanner 设计

禁止递归扫描目录。

必须使用：

```text
Task Queue
    ↓
Worker Pool
    ↓
OpenList API
    ↓
Result Queue
    ↓
SQLite Writer
```

默认：

```text
Workers = 32
QueueSize = 10000
```

支持配置修改。

目标：

```text
100000+
媒体文件
```

---

# SQLite

必须创建：

## media_files

字段：

```sql
id
storage
path
name
size
is_dir
modified
created_at
```

## scan_tasks

字段：

```sql
path
status
updated_at
```

## tmdb_cache

字段：

```sql
normalized_name
tmdb_id
media_type
updated_at
```

---

# 数据写入规则

禁止逐条写入。

必须：

```text
1000条提交一次
或者
5秒提交一次
```

---

# 媒体名称标准化

删除：

- 2160P
- 1080P
- 720P
- 4K
- HDR
- DV
- BluRay
- WEB-DL
- WEBRip
- x265
- H265
- AAC

统一：

```text
.
-
_
```

替换为空格。

---

# 电视剧识别

支持：

```text
S01E01
S01E02

EP01
EP02

第01集
第02集

第1季第1集
```

统一格式：

```text
庆余年_S01E01
```

---

# 重复检测

第一层：

```text
标准化名称
```

第二层：

```text
文件大小误差 < 1%
```

第三层：

```text
TMDB ID
```

---

# 存储优先级

```text
本地 > 天翼 > 夸克
```

输出：

```text
Keep
Delete
```

---

# TMDB

实现：

```go
SearchMovie()
SearchTV()
```

要求：

- 中文搜索
- 英文搜索
- SQLite缓存

禁止重复请求同一名称。

---

# HTML 报告

生成：

```text
report.html
```

内容：

- 重复电影
- 重复电视剧
- 删除建议
- 存储统计

使用：

```go
html/template
```

禁止前端框架。

---

# 自动清理

默认：

```text
Dry Run
```

生成：

```text
cleanup_plan.json
```

只有：

```bash
--apply
```

才允许调用：

```text
/api/fs/remove
```

执行删除。

---

# 日志

所有模块必须记录：

- Start
- Finish
- Error
- Statistics

格式：

```text
JSON Structured Log
```

---

# 性能目标

```text
100000+
文件
```

内存：

```text
< 2GB
```

Worker：

```text
32~128
```

SQLite：

```text
Batch Write Only
```

---

# 测试

必须：

- normalize_test.go
- duplicate_test.go
- sqlite_test.go
- scanner_test.go

覆盖率：

```text
70%+
```

---

# 开发顺序

严格执行：

1. Config
2. Logger
3. OpenList SDK
4. Scanner
5. SQLite
6. Normalize
7. Duplicate
8. TMDB
9. Report
10. Cleanup
11. Docker

禁止跳阶段开发。

每完成一个阶段：

- 编译
- 测试
- 修复问题
- Git Commit

然后进入下一阶段。