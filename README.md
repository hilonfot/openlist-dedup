# OpenList 媒体资源去重系统

生产级媒体巡检工具，用于扫描 OpenList 中的夸克网盘、天翼云盘和本地存储资源，建立统一媒体索引，并自动识别重复电影和电视剧资源。

## 功能

- **并发扫描** — BFS 广度优先，Worker Pool 并发 (默认 32 Worker)
- **多存储支持** — 夸克网盘 / 天翼云盘 / 本地存储
- **SQLite 索引** — 批量写入 (1000条/5秒自动Flush)，WAL 模式
- **媒体标准化** — 自动清理质量标签 (4K/HDR/BluRay/x265...)，识别剧集信息
- **TMDB 匹配** — 多语言搜索 (中文优先)，SQLite 缓存
- **重复检测** — 区分电影 / 电视剧，支持文件级、剧集级、电视剧文件夹级去重
- **HTML 报告** — 可视化去重报告，Keep/Delete 建议
- **自动清理** — Dry Run 模式，--apply 执行删除

## 识别与去重规则

系统会先对文件名做标准化，再根据电影和电视剧的不同结构生成去重 Key。

### 电影

电影按以下信息识别同一资源：

```text
标准化片名 + 年份 + 文件大小容差
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

### 保留优先级

当多个重复文件或目录都可作为保留对象时，默认存储优先级为：

```text
local > tianyi > quark
```

同一存储内的电视剧文件夹级重复会优先按完整度选择，而不是按路径名称选择。

## 快速开始

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
```

### 命令行参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--config` | 配置文件路径 | `configs/config.yaml` |
| `--db` | SQLite 数据库路径 | `data/media.db` |
| `--workers` | 扫描并发数 | `32` |
| `--scan` | 执行扫描 | |
| `--report` | 生成 HTML 报告 | |
| `--report-path` | 报告输出路径 | `report.html` |
| `--cleanup` | 创建清理计划 | |
| `--plan-path` | 计划输出路径 | `cleanup_plan.json` |
| `--apply` | 执行实际删除 (默认 Dry Run) | |

## 配置

编辑 `configs/config.yaml`:

```yaml
openlist:
  url: "http://localhost:5244"
  password: ""
  timeout: 30
  retry_max: 3

scanner:
  workers: 32
  queue_size: 10000

database:
  path: "data/media.db"

log:
  level: "info"
```

环境变量可覆盖配置 (`OPENLIST_URL`, `SCANNER_WORKERS`, `TMDB_API_KEY` 等)。

## 项目结构

```
├── cmd/openlist/        # 入口点
├── internal/
│   ├── config/          # 配置加载
│   ├── logger/          # JSON 结构化日志
│   ├── openlist/        # OpenList SDK
│   ├── scanner/         # 并发扫描器
│   ├── repository/      # SQLite 数据层
│   ├── media/           # 媒体名称标准化
│   ├── duplicate/       # 重复检测
│   ├── tmdb/            # TMDB 集成
│   ├── report/          # HTML 报告
│   └── cleanup/         # 自动清理
├── configs/             # 配置文件
├── data/                # 数据目录
├── Dockerfile           # 多阶段构建
├── docker-compose.yml   # Docker Compose
├── Makefile             # 构建工具
└── docs/                # 文档
```

## 技术栈

- Go 1.24+
- SQLite (modernc.org/sqlite, 纯 Go, 无需 CGO)
- Docker / Docker Compose
- 最小依赖原则 (仅 yaml.v3 + modernc.org/sqlite)

## License

MIT
