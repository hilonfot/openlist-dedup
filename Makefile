.PHONY: build test run clean docker docker-build docker-run lint help

APP_NAME   := openlist
BUILD_DIR  := build
GO_FLAGS   := -ldflags="-s -w"
GO_PKG     := ./cmd/openlist/

help:
	@echo "OpenList 媒体去重系统"
	@echo ""
	@echo "Usage:"
	@echo "  make build      编译二进制"
	@echo "  make test       运行全部测试"
	@echo "  make run        运行扫描 (默认模式)"
	@echo "  make docker     构建并运行 Docker"
	@echo "  make clean      清理构建产物"

build:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build $(GO_FLAGS) -o $(BUILD_DIR)/$(APP_NAME) $(GO_PKG)
	@echo "Build: $(BUILD_DIR)/$(APP_NAME)"

test:
	go test ./... -cover -count=1 -timeout 120s

run: build
	./$(BUILD_DIR)/$(APP_NAME) --scan --clear-data

run-report: build
	./$(BUILD_DIR)/$(APP_NAME) --report --report-path report.html

run-cleanup: build
	./$(BUILD_DIR)/$(APP_NAME) --cleanup --plan-path cleanup_plan.json

run-apply: build
	./$(BUILD_DIR)/$(APP_NAME) --cleanup --apply --plan-path cleanup_plan.json

docker-build:
	docker build -t $(APP_NAME):latest .

docker-run:
	docker compose up --build

docker:
	docker-build docker-run

lint:
	@go vet ./...
	@echo "Vet passed"

clean:
	rm -rf $(BUILD_DIR)/
	rm -f report.html cleanup_plan.json
