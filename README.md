# OpenList 媒体资源去重系统

生产级媒体巡检工具，用于扫描 OpenList 中的夸克网盘、天翼云盘和本地存储资源，建立统一媒体索引，并自动识别重复电影和电视剧资源。

## 功能

- **并发扫描** — BFS 广度优先，Worker Pool 并发 (默认 32 Worker)
- **多存储支持** — 夸克网盘 / 天翼云盘 / 本地存储
- **SQLite 索引** — 批量写入 (1000条/5秒自动Flush)，WAL 模式
- **媒体标准化** — 自动清理质量标签 (4K/HDR/BluRay/x265...)，识别剧集信息
- **目录上下文识别** — 文件名无标题时，自动从父目录名提取剧名/电影名
- **质量指纹评分** — 识别分辨率/编码/HDR/音质/来源，质量优先保留
- **TMDB 匹配** — 多语言搜索 (中文优先)，SQLite 缓存，本地映射 fallback
- **重复检测** — 区分电影 / 电视剧，支持文件级、剧集级、电视剧文件夹级去重
- **HTML 报告** — 可视化去重报告，Keep/Delete 建议
- **自动清理** — Dry Run 模式，`--apply` 执行删除，支持恢复指南
- **Token 自动刷新** — 检测 401 自动重登录，适合长时间扫描

## 识别与去重规则

系统会先对文件名做标准化，再根据电影和电视剧的不同结构生成去重 Key。

### 电影

电影按以下信息识别同一资源：

```text
标准化片名 + 年份 + 质量指纹 + 文件大小容差
```

示例：

```text
Avatar.2022.1080p.BluRay.x265.mkv
Avatar.2022.2160p.BluRay.x265.mkv
```

如果片名和年份相同，但文件大小差异超过容差，会被视为不同版本，不直接判定为重复。

同名不同年份会被视为不同电影，例如：

```text
Movie.2020.1080p.mkv
Movie.2021.1080p.mkv
```

### 质量优先保留

当同一媒体存在多个版本时，系统按**质量评分**优先保留，存储类型作为平局决胜：

```text
质量评分 = 分辨率分 + 编码分 + HDR分 + 音质分 + 来源分

4K REMUX TrueHD Atmos > 4K WEB-DL AAC > 1080p BluRay DTS > 1080p WEBRip AAC
```

同名同质量时：`local > tianyi > quark`

### 电视剧

电视剧按具体剧集识别文件级重复：

```text
剧名 + 年份 + 季集号(SxxExx) + 文件大小容差
```

因此同一部剧里的不同集不会互相误判为重复：

```text
Show.S01E01.mkv
Show.S01E02.mkv
Show.S01E03.mkv
```

如果文件名只有集数，例如：

```text
绝命毒师/Season 1/01.mkv
绝命毒师/S01/01.mkv
```

系统会从目录名提取剧名，并从季目录和文件名补全 `S01E01`。

### 电视剧文件夹级重复

同一个存储内，如果同一部电视剧出现在多个根目录，也会被筛选为重复资源，即使两个目录里的集数不完全一致。

示例：

```text
/quark/电视剧/狂飙/
  狂飙.S01E01.mkv
  狂飙.S01E02.mkv
  狂飙.S01E03.mkv

/quark/来自分享/狂飙 4K/
  狂飙.S01E01.mkv
  狂飙.S01E02.mkv
```

系统会生成电视剧文件夹级重复组，优先保留：

1. 集数更多的目录
2. 文件数更多的目录
3. 总容量更大的目录

其余重复目录会在报告和清理计划中标记为 `Delete`。

## 快速开始

### 配置 (.env 文件)

```bash
# 复制配置模板
cp .env.example .env

# 编辑 .env 填入实际值
vim .env
```

`.env` 文件示例：

```bash
OPENLIST_URL=http://192.168.123.7:5255
OPENLIST_USERNAME=admin
OPENLIST_PASSWORD=your_password
SCANNER_WORKERS=32
DATABASE_PATH=data/media.db
TMDB_API_KEY=your_tmdb_key
STORAGE_LOCAL_PATHS=/xunlei
STORAGE_QUARK_PATHS=/夸克网盘/电影,/夸克网盘/电视剧
STORAGE_TIANYI_PATHS=/天翼云盘
```

环境变量可覆盖 `.env` 中的值，系统级 env 优先级最高：
`.env 文件` → `OS 环境变量`

### 编译与运行

```bash
# 编译
make build

# 运行扫描
make run

# 生成去重报告
make run-report

# 生成清理计划 (Dry Run)
make run-cleanup

# 执行清理
make run-apply
```

### Docker 部署

```bash
# 构建并启动
make docker

# 或分步执行
docker build -t openlist:latest .
docker compose up
```

### 手动运行

```bash
# 扫描媒体文件
./build/openlist --scan

# 生成 HTML 报告
./build/openlist --report --report-path report.html

# 创建清理计划
./build/openlist --cleanup --plan-path cleanup_plan.json

# 执行清理 (谨慎!)
./build/openlist --cleanup --apply

# 查看恢复指南 (上次清理结果)
./build/openlist --restore
```

### 命令行参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--config` | .env 配置文件路径 | `.env` |
| `--db` | SQLite 数据库路径 | `data/media.db` |
| `--workers` | 扫描并发数 | `32` |
| `--scan` | 执行扫描 | |
| `--report` | 生成 HTML 报告 | |
| `--report-path` | 报告输出路径 | `report.html` |
| `--cleanup` | 创建清理计划 | |
| `--plan-path` | 计划输出路径 | `cleanup_plan.json` |
| `--apply` | 执行实际删除 (默认 Dry Run) | |
| `--restore` | 加载上次清理结果，显示恢复指南 | |
| `--clear-data` | 扫描前清理旧数据 (保留 TMDB 缓存) | |

## 本地映射 (TMDB Fallback)

国产影视 TMDB 覆盖不足时，可在 `configs/mapping.yaml` 中添加映射：

```yaml
# 电影
流浪地球2:
  tmdb_id: 842675
  type: movie

# 电视剧
庆余年:
  tmdb_id: 94307
  type: tv
```

## 项目结构

```
├── cmd/openlist/        # 入口点
├── internal/
│   ├── config/          # 配置加载 (.env + OS env)
│   ├── logger/          # JSON 结构化日志
│   ├── openlist/        # OpenList SDK (token 自动刷新)
│   ├── scanner/         # 并发扫描器 (异步种子投递)
│   ├── repository/      # SQLite 数据层 (WAL 模式)
│   ├── media/           # 媒体标准化 + 质量指纹
│   ├── duplicate/       # 重复检测 (质量优先)
│   ├── tmdb/            # TMDB 集成 + 本地映射 fallback
│   ├── report/          # HTML 报告
│   └── cleanup/         # 自动清理 + 恢复指南
├── configs/             # 映射配置 (mapping.yaml)
├── data/                # 数据目录 (SQLite DB)
├── Dockerfile           # 多阶段构建
├── docker-compose.yml   # Docker Compose
├── Makefile             # 构建工具
├── .env.example         # 环境变量模板
└── docs/                # 文档
```

## 技术栈

- Go 1.24+
- SQLite (`modernc.org/sqlite`, 纯 Go, 无需 CGO)
- Docker / Docker Compose
- 最小依赖原则 (仅 `gopkg.in/yaml.v3` + `modernc.org/sqlite`)

## License

MIT
