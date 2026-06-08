# OpenList 媒体资源去重系统

生产级媒体巡检工具，用于扫描 OpenList 中的夸克网盘、天翼云盘和本地存储资源，建立统一媒体索引，并自动识别重复电影和电视剧资源。

## 功能

- **并发扫描** — BFS 广度优先，Worker Pool 并发 (默认 32 Worker)
- **多存储支持** — 夸克网盘 / 天翼云盘 / 本地存储
- **SQLite 索引** — 批量写入 (1000条/5秒自动Flush)，WAL 模式
- **媒体标准化** — 自动清理质量标签 (4K/HDR/BluRay/x265...)，识别剧集信息
- **TMDB 匹配** — 多语言搜索 (中文优先)，SQLite 缓存
- **重复检测** — 三层去重 (名称 + 大小 + TMDB ID)
- **HTML 报告** — 可视化去重报告，Keep/Delete 建议
- **自动清理** — Dry Run 模式，--apply 执行删除

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
