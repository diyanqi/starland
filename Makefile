SHELL := /bin/sh

.PHONY: help install dev-web dev-api build run docker clean

help:
	@echo "拾光星岛 · 常用命令"
	@echo "  make install   安装前端依赖"
	@echo "  make dev-web   启动 Astro 开发服务器 (4321，API 代理到 8080)"
	@echo "  make dev-api   启动 Go 服务 (8080)"
	@echo "  make build     构建前端 + 嵌入并编译单文件二进制到 bin/"
	@echo "  make run       构建并运行 (读取 backend/.env)"
	@echo "  make docker    构建并启动 docker compose 全套"
	@echo "  make clean     清理构建产物"

install:
	cd frontend && npm install

dev-web:
	cd frontend && npm run dev

dev-api:
	cd backend && go run .

build:
	cd frontend && npm run build
	rm -rf backend/web/dist
	cp -R frontend/dist backend/web/dist
	mkdir -p bin
	cd backend && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ../bin/shiguang-island .
	@echo "✅ 已生成 bin/shiguang-island"

run: build
	cd backend && ../bin/shiguang-island

docker:
	docker compose up -d --build

clean:
	rm -rf bin frontend/dist backend/web/dist
