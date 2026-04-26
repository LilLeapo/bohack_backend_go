# BoHack backend dev/test Makefile
# 默认目标：用 sqlite 起服务，邮件配置优先读取 .env。

SHELL := /usr/bin/env bash

-include .env

# ---- 可覆盖参数 ----
HOST                              ?= 127.0.0.1
PORT                              ?= 8080
SQLITE_PATH                       ?= ./bohack.dev.sqlite
FRONTEND_DIR                      ?= ./bohack_2026_web_design
FRONTEND_PORT                     ?= 5173
FRONTEND_API_BASE_URL             ?= http://$(HOST):$(PORT)
JWT_SECRET                        ?= dev-secret-change-me
DEFAULT_EVENT_SLUG                ?= bohack-2026
DEFAULT_EVENT_TITLE               ?= BoHack 2026
ALLOWED_ORIGINS                   ?= *
ATTACHMENT_DIR                    ?= ./storage/registration_attachments
MAX_UPLOAD_MB                     ?= 20
ACCESS_TOKEN_TTL_MINUTES          ?= 720
VERIFICATION_CODE_EXPIRE_MINUTES  ?= 10
VERIFICATION_CODE_MIN_INTERVAL_SECONDS ?= 60
REQUIRE_REGISTER_VERIFICATION     ?= true
MAIL_MODE                         ?= smtp
SMTP_HOST                         ?=
SMTP_PORT                         ?= 587
SMTP_USERNAME                     ?=
SMTP_PASSWORD                     ?=
SMTP_FROM                         ?=

# 公共环境。所有值用引号，避免含空格的标题被拆成命令。
DEV_ENV = \
	DB_DRIVER=sqlite \
	SQLITE_PATH='$(SQLITE_PATH)' \
	MAIL_MODE='$(MAIL_MODE)' \
	SMTP_HOST='$(SMTP_HOST)' \
	SMTP_PORT='$(SMTP_PORT)' \
	SMTP_USERNAME='$(SMTP_USERNAME)' \
	SMTP_PASSWORD='$(SMTP_PASSWORD)' \
	SMTP_FROM='$(SMTP_FROM)' \
	JWT_SECRET='$(JWT_SECRET)' \
	HOST='$(HOST)' \
	PORT='$(PORT)' \
	DEFAULT_EVENT_SLUG='$(DEFAULT_EVENT_SLUG)' \
	DEFAULT_EVENT_TITLE='$(DEFAULT_EVENT_TITLE)' \
	ALLOWED_ORIGINS='$(ALLOWED_ORIGINS)' \
	ATTACHMENT_DIR='$(ATTACHMENT_DIR)' \
	MAX_UPLOAD_MB='$(MAX_UPLOAD_MB)' \
	ACCESS_TOKEN_TTL_MINUTES='$(ACCESS_TOKEN_TTL_MINUTES)' \
	VERIFICATION_CODE_EXPIRE_MINUTES='$(VERIFICATION_CODE_EXPIRE_MINUTES)' \
	VERIFICATION_CODE_MIN_INTERVAL_SECONDS='$(VERIFICATION_CODE_MIN_INTERVAL_SECONDS)' \
	REQUIRE_REGISTER_VERIFICATION='$(REQUIRE_REGISTER_VERIFICATION)'

.PHONY: help dev run dev-verify dev-all dev-frontend frontend-install build tidy clean reset-db storage e2e test fmt vet

help:
	@echo "Available targets:"
	@echo "  make dev-all       - run backend + frontend together (Ctrl+C stops both)"
	@echo "  make dev           - run backend only (sqlite + configured mailer)"
	@echo "  make dev-verify    - same as dev, but REQUIRE_REGISTER_VERIFICATION=true"
	@echo "  make dev-frontend  - run frontend dev server only"
	@echo "  make frontend-install - npm install in $(FRONTEND_DIR)"
	@echo "  make build         - go build ./cmd/server -> ./bin/server"
	@echo "  make tidy          - go mod tidy"
	@echo "  make fmt           - gofmt -w ."
	@echo "  make vet           - go vet ./..."
	@echo "  make test          - go test ./..."
	@echo "  make e2e           - go test ./... -tags=e2e -count=1"
	@echo "  make reset-db      - delete the local sqlite file ($(SQLITE_PATH))"
	@echo "  make storage       - create attachment storage dir"
	@echo "  make clean         - reset-db + remove build artifacts"
	@echo ""
	@echo "Override any var, e.g.: make dev PORT=9090 SQLITE_PATH=./tmp.db"
	@echo "                       make dev-all FRONTEND_PORT=3000"

dev: storage
	$(DEV_ENV) go run ./cmd/server

run: dev

dev-verify: storage
	$(DEV_ENV) REQUIRE_REGISTER_VERIFICATION=true go run ./cmd/server

frontend-install:
	cd "$(FRONTEND_DIR)" && npm install

dev-frontend:
	@if [ ! -d "$(FRONTEND_DIR)/node_modules" ]; then \
		echo "node_modules missing, running npm install..."; \
		cd "$(FRONTEND_DIR)" && npm install; \
	fi
	cd "$(FRONTEND_DIR)" && VITE_API_BASE_URL='$(FRONTEND_API_BASE_URL)' npm run dev -- --host '$(HOST)' --port '$(FRONTEND_PORT)'

# 一同启动后端 + 前端，前台跑前端，后端在后台。Ctrl+C 时清理两边。
dev-all: storage
	@if [ ! -d "$(FRONTEND_DIR)/node_modules" ]; then \
		echo "node_modules missing, running npm install..."; \
		cd "$(FRONTEND_DIR)" && npm install; \
	fi
	@trap 'echo; echo "stopping..."; kill 0' INT TERM EXIT; \
		( $(DEV_ENV) go run ./cmd/server ) & \
		BACKEND_PID=$$!; \
		echo "backend pid=$$BACKEND_PID listening on $(HOST):$(PORT)"; \
		sleep 1; \
		( cd "$(FRONTEND_DIR)" && VITE_API_BASE_URL='$(FRONTEND_API_BASE_URL)' npm run dev -- --host '$(HOST)' --port '$(FRONTEND_PORT)' ) & \
		FRONTEND_PID=$$!; \
		echo "frontend pid=$$FRONTEND_PID listening on $(HOST):$(FRONTEND_PORT)"; \
		wait

build:
	go build -o ./bin/server ./cmd/server

tidy:
	go mod tidy

fmt:
	gofmt -w .

vet:
	go vet ./...

test:
	$(DEV_ENV) go test ./... -count=1

e2e:
	$(DEV_ENV) go test ./... -tags=e2e -count=1

storage:
	@mkdir -p "$(ATTACHMENT_DIR)"

reset-db:
	@rm -f "$(SQLITE_PATH)"
	@echo "removed $(SQLITE_PATH)"

clean: reset-db
	@rm -rf ./bin
