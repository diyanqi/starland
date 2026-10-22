# ---------- 1. 构建 Astro 前端 ----------
FROM node:22-alpine AS web

WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# ---------- 2. 编译 Go 并嵌入前端 ----------
FROM golang:1.24-alpine AS build

WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./
COPY --from=web /app/frontend/dist ./web/dist

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/island .

# ---------- 3. 运行镜像 ----------
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 island

WORKDIR /app
COPY --from=build /out/island /app/island
RUN mkdir -p /app/uploads && chown island:island /app/uploads

USER island
ENV PORT=8080 \
    UPLOAD_DIR=/app/uploads

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/api/health >/dev/null || exit 1

ENTRYPOINT ["/app/island"]
