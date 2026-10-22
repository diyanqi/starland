# 拾光星岛 · Shiguang Island

一个班级专属的轻互动网站：**照片墙**收藏日常瞬间，**文字岛**收留夸夸、吐槽和感悟。移动端优先，浏览器直接访问，不需要注册登录。

技术栈：

| 层 | 选型 |
| --- | --- |
| 前端 | Astro 5（纯静态输出）+ Tailwind CSS 4 + 原生 TypeScript |
| 后端 | Go 1.24 标准库 `net/http` + chi 路由 |
| 数据库 | PostgreSQL 13+（pgx v5 连接池） |
| 部署 | Astro `dist` 通过 `embed` 编进 Go 二进制，同源单文件部署 |

---

## 1. 目录结构

```text
shiguang-island/
├── frontend/                      # Astro 前端（构建产物：frontend/dist）
│   ├── public/favicon.svg
│   ├── src/
│   │   ├── components/Icon.astro
│   │   ├── layouts/Layout.astro   # 站点头部、移动端底部导航、页脚
│   │   ├── lib/client/api.ts      # fetch 封装、类型、clientId、错误处理
│   │   ├── lib/client/ui.ts       # 转义、时间格式化、toast
│   │   ├── pages/index.astro      # 首页（最近照片 / 最近留言）
│   │   ├── pages/photos.astro     # 照片墙：上传、瀑布流、点赞、灯箱
│   │   ├── pages/island.astro     # 文字岛：发布、筛选、单层回复
│   │   ├── pages/admin.astro      # 管理台：令牌进入、删除内容
│   │   └── styles/global.css      # Tailwind 主题与组件样式
│   ├── astro.config.mjs
│   └── package.json
├── backend/                       # Go 服务
│   ├── main.go                    # 启动、优雅退出
│   ├── static.go                  # embed 前端 + 静态文件/SPA fallback
│   ├── web/dist/                  # 前端构建产物（由 make build 复制进来）
│   ├── migrations/
│   │   ├── embed.go               # 启动时自动执行迁移（带 advisory lock）
│   │   └── 001_init.sql           # 建表语句
│   ├── internal/
│   │   ├── config/config.go       # 环境变量 + 极简 .env 读取
│   │   ├── store/                 # pgxpool、SQL 查询、领域模型
│   │   └── httpapi/               # 路由、中间件、上传、REST handlers
│   ├── .env.example
│   └── go.mod
├── Dockerfile                     # 三段式构建：Astro → Go → Alpine
├── docker-compose.yml             # app + PostgreSQL 16
├── Makefile
└── README.md
```

---

## 2. 部署架构

```text
                       ┌──────────────────────────── Go 单进程 :8080 ───────────────────────────┐
浏览器 ──HTTPS──> Nginx │  /api/*      → chi handlers → pgxpool → PostgreSQL                    │
                       │  /uploads/*  → 本地磁盘 UPLOAD_DIR                                   │
                       │  /*          → embed.FS(web/dist) + 缓存头 + SPA fallback             │
                       └──────────────────────────────────────────────────────────────────────┘
```

- 前端 API 请求全部使用相对路径（`/api/...`），前后端**天然同源**，不需要 CORS。
- 生产环境只需部署一个 Go 二进制 + 一个 PostgreSQL + 一个上传目录。
- 开发环境由 Astro 的 `server.proxy` 把 `/api`、`/uploads` 转发到 `localhost:8080`，行为与生产一致。

---

## 3. 快速开始

### 3.1 依赖

- Go >= 1.24
- Node.js >= 20
- PostgreSQL >= 13，或者 Docker

### 3.2 启动数据库

```bash
# 方式 A：只用 compose 里的数据库
docker compose up -d db

# 方式 B：本机已有 PostgreSQL
createdb shiguang
```

### 3.3 启动后端

```bash
cp backend/.env.example backend/.env
# 编辑 backend/.env：至少确认 DATABASE_URL / ADMIN_TOKEN

cd backend
go mod download
go run .
```

服务启动时会自动执行 `backend/migrations/*.sql`，并通过 `schema_migrations` 表记录已执行的版本。默认监听 `:8080`。

### 3.4 启动前端开发服务器

```bash
cd frontend
npm install
npm run dev
```

打开 <http://localhost:4321>。`/api` 与 `/uploads` 会自动代理到 Go 服务（8080），不需要手配 CORS。

### 3.5 生产构建（单文件）

```bash
make build
# 等价于：构建 Astro → 复制 dist 到 backend/web/dist → go build 嵌入
```

生成 `bin/shiguang-island`。运行：

```bash
cd backend
DATABASE_URL="postgres://user:pass@127.0.0.1:5432/shiguang?sslmode=disable" \
ADMIN_TOKEN="your-long-admin-token" \
UPLOAD_DIR="./uploads" \
../bin/shiguang-island
```

> 注意：`go build` 前 `backend/web/dist` 必须存在，否则 `//go:embed` 编译会失败。使用 `make build` 或 Dockerfile 都会自动处理。

### 3.6 Docker 一键部署

```bash
cp .env.example .env        # 修改 POSTGRES_PASSWORD 和 ADMIN_TOKEN
docker compose up -d --build
```

访问 <http://localhost:8080>。数据库与上传文件分别持久化在 `pgdata`、`uploads` 两个 volume 中。

---

## 4. 环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `ADDR` | `:8080` | 监听地址；未设置时兼容平台注入的 `PORT` |
| `PORT` | — | 仅当 `ADDR` 未设置时生效 |
| `DATABASE_URL` | 必填 | PostgreSQL 连接串 |
| `UPLOAD_DIR` | `./uploads` | 图片落盘目录，需可写 |
| `ADMIN_TOKEN` | 空 | 管理令牌；为空时管理接口返回 503；设置后至少 8 位 |
| `MAX_UPLOAD_MB` | `8` | 单张图片大小上限，1–50 |
| `TRUST_PROXY` | `false` | 位于 Nginx/CDN 后时设为 `true`，才信任 `X-Forwarded-For` |

---

## 5. 数据库设计

完整建表语句见 [`backend/migrations/001_init.sql`](backend/migrations/001_init.sql)：

```sql
CREATE TABLE photos (
    id         BIGSERIAL PRIMARY KEY,
    url        TEXT         NOT NULL,
    caption    VARCHAR(200) NOT NULL DEFAULT '',
    nickname   VARCHAR(24)  NOT NULL DEFAULT '匿名星友',
    likes      INTEGER      NOT NULL DEFAULT 0 CHECK (likes >= 0),
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE messages (
    id         BIGSERIAL PRIMARY KEY,
    content    VARCHAR(2000) NOT NULL CHECK (length(btrim(content)) > 0),
    type       VARCHAR(16)   NOT NULL CHECK (type IN ('praise', 'rant', 'thought')),
    nickname   VARCHAR(24)   NOT NULL DEFAULT '匿名星友',
    created_at TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE TABLE replies (
    id         BIGSERIAL PRIMARY KEY,
    message_id BIGINT        NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
    content    VARCHAR(1000) NOT NULL CHECK (length(btrim(content)) > 0),
    nickname   VARCHAR(24)   NOT NULL DEFAULT '匿名星友',
    created_at TIMESTAMPTZ   NOT NULL DEFAULT now()
);

-- 点赞去重：同一浏览器对同一照片只计一次
CREATE TABLE photo_likes (
    photo_id   BIGINT      NOT NULL REFERENCES photos (id) ON DELETE CASCADE,
    client_id  VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (photo_id, client_id)
);

CREATE INDEX photos_created_at_idx ON photos (created_at DESC, id DESC);
CREATE INDEX messages_created_at_idx ON messages (created_at DESC, id DESC);
CREATE INDEX messages_type_created_at_idx ON messages (type, created_at DESC);
CREATE INDEX replies_message_idx ON replies (message_id, created_at ASC);
```

设计要点：

- `replies.message_id` 外键 `ON DELETE CASCADE`，管理端删留言时回复自动清理。
- 列表按自增 `id` 做游标分页（`WHERE id < cursor`），比 OFFSET 稳定，插入新内容不会导致重复/漏项。
- 点赞计数放在 `photos.likes` 上，`photo_likes` 只负责去重，避免每次列表都聚合统计。
- 类型用 `CHECK` 约束兜底，即使绕过 API 也不会写入脏数据。

---

## 6. API

统一响应格式：

```json
{ "data": { ... } }
{ "error": { "code": "invalid_content", "message": "内容需在 1-2000 字之间" } }
```

| 方法 | 路径 | 说明 | 鉴权 |
| --- | --- | --- | --- |
| GET | `/api/health` | 健康检查（含数据库 Ping） | 否 |
| GET | `/api/photos?limit=24&cursor=<id>` | 照片列表，倒序、游标分页，含 `liked` | 否 |
| POST | `/api/photos` | 上传照片（`multipart/form-data`：`image`、`caption`、`nickname`） | 否 |
| POST | `/api/photos/{id}/like` | 点赞（按 `X-Client-Id` 幂等） | 否 |
| GET | `/api/messages?type=praise&limit=20&cursor=<id>` | 留言列表，可按类型筛选 | 否 |
| POST | `/api/messages` | 发布留言 `{content,type,nickname}` | 否 |
| GET | `/api/messages/{id}/replies` | 某条留言的回复 | 否 |
| POST | `/api/messages/{id}/replies` | 发布单层回复 `{content,nickname}` | 否 |
| GET | `/api/admin/ping` | 验证管理令牌 | `X-Admin-Token` |
| DELETE | `/api/admin/photos/{id}` | 删除照片（同时删除磁盘文件） | `X-Admin-Token` |
| DELETE | `/api/admin/messages/{id}` | 删除留言（级联删除回复） | `X-Admin-Token` |
| DELETE | `/api/admin/replies/{id}` | 删除单条回复 | `X-Admin-Token` |

管理令牌支持两种传法：`X-Admin-Token: <token>` 或 `Authorization: Bearer <token>`。前端管理台把令牌放在 `sessionStorage`，关闭标签页即失效。

---

## 7. 关键实现说明

### 7.1 用 embed 嵌入前端并处理刷新 404

`backend/static.go`：

```go
//go:embed all:web/dist
var frontendFS embed.FS

func newStaticHandler() (http.Handler, error) {
    dist, err := fs.Sub(frontendFS, "web/dist")
    if err != nil {
        return nil, err
    }
    return &staticHandler{fsys: dist}, nil
}
```

路由兜底（`internal/httpapi/router.go`）：

```go
r.Handle("/uploads/*", http.HandlerFunc(h.serveUpload))
r.Route("/api", func(api chi.Router) { /* REST 路由 */ })
r.NotFound(static.ServeHTTP)   // 其余全部交给内嵌静态文件
```

静态处理逻辑：

1. `/` → `index.html`；
2. 依次尝试 `photos`、`photos/index.html`、`photos.html`（兼容 Astro 的 directory / file 两种输出）；
3. 无扩展名且 `Accept: text/html` → 回退 `index.html`，避免直接刷新深层链接出现 404；
4. 仍然找不到 → 返回 `404.html` 并保持 HTTP 404。

缓存策略：`/_astro/*`（带内容 hash）设置一年 immutable，HTML 设 `no-cache`，`/uploads/*` 同样一年 immutable。

> 如果以后改成真正的 SPA（如 React Router），“无扩展名回退 index.html”就是标准做法；当前 Astro 是多页面静态站，这个分支只是额外保险。

### 7.2 图片上传

`internal/httpapi/uploads.go` 做了这些校验：

- `http.MaxBytesReader` 限制请求体，`MAX_UPLOAD_MB` 限制单文件；
- 读取前 512 字节用 `http.DetectContentType` **按真实内容**判断类型，只放行 JPG / PNG / GIF / WebP（拒绝 SVG，避免存储型 XSS）；
- 文件名由服务端生成：`日期-随机十六进制.扩展名`，不信任用户文件名；
- `O_EXCL` 创建文件，写入超限或数据库失败时删除临时文件；
- 访问路径固定为 `/uploads/<name>`，仅接受白名单正则，禁止目录穿越与目录列表。

图片默认落本地磁盘。如果要用图床（对象存储）也很简单：

- **方案 A（推荐）**：前端先直传 OSS/S3（服务端签发预签名 URL 或 STS），把返回的公网 URL 作为 `url` 提交给 `POST /api/photos`；
- **方案 B**：Go 收到文件后转存对象存储，`photos.url` 存 CDN 地址，删除照片时调用对象存储删除接口；
- 数据库字段 `url` 是 `TEXT`，既可存 `/uploads/xxx.png`，也可存 `https://cdn.example.com/xxx.webp`，前端无需改动。

### 7.3 点赞幂等

浏览器首次访问生成 `crypto.randomUUID()` 存入 `localStorage`，请求时带 `X-Client-Id`。服务端在同一事务里：

1. `INSERT INTO photo_likes ... ON CONFLICT DO NOTHING RETURNING photo_id`；
2. 插入成功 → `UPDATE photos SET likes = likes + 1`；
3. 冲突（已点过）→ 只读当前计数，不再 +1。

缺失或非法 `X-Client-Id` 时退化为 IP 标识，保证不会因客户端异常而无限刷赞。生产多实例环境下如果想做全局限流，把内存限流器替换成 Redis 即可。

### 7.4 前端数据交互

`src/lib/client/api.ts` 统一封装：

```ts
export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set('X-Client-Id', getClientId());
  if (typeof init.body === 'string') headers.set('Content-Type', 'application/json');

  const response = await fetch(path, { ...init, headers });
  return parse<T>(response);
}
```

- 所有用户内容渲染前经过 `escapeHtml()`，避免 XSS；
- 列表使用游标“加载更多”，按钮在无更多数据时隐藏；
- 点赞、发布、回复都有 disabled / loading / toast 错误提示；
- 上传失败、网络异常、后端校验错误都会显示服务端返回的中文 message。

### 7.5 UI 设计

- 色板：米白 `#fbf7ef`、墨色 `#2c3742`、海松绿 `#6d9e96`、星光金 `#dfae5c`、晚霞粉 `#dfa39a`；
- 标题使用系统中文字体宋体栈，正文使用无衬线栈，不依赖外部字体 CDN；
- 移动端底部三栏导航 + 安全区适配，桌面端顶部胶囊导航；
- 照片墙用 CSS `columns` 实现轻量瀑布流，无需 JS 布局库；
- 卡片使用半透明背景、细边框与大圆角，保持清新、文艺、轻量。

---

## 8. 部署与运维建议

### Nginx 反向代理

```nginx
server {
    listen 443 ssl http2;
    server_name island.example.com;

    client_max_body_size 12m;        # 要大于 MAX_UPLOAD_MB

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 60s;
    }
}
```

此时把后端 `TRUST_PROXY=true`，限流和点赞兜底才会使用真实客户端 IP。

### 数据与备份

- 上传目录必须挂载持久化 volume，并与数据库一起备份；
- 备份命令示例：`pg_dump -Fc shiguang > shiguang.dump`，上传目录用 `rsync`/对象存储同步；
- 数据库迁移在服务启动时自动执行，回滚请提前备份。

### 安全清单

- 一定修改默认 `ADMIN_TOKEN`，并通过 HTTPS 传输；
- 已限制上传类型、大小、文件名与服务端嗅探；生产可在 Nginx 再加 `client_max_body_size` 与速率限制；
- 用户内容全部转义渲染，管理端删除是唯一的内容治理入口；
- 单实例内存限流足够班级规模；横向扩容时把限流与点赞去重迁移到 Redis。

### 可扩展方向

- 图片压缩 / 自动转 WebP / 去除 EXIF（`bimg`、`imaging` 或上传后异步任务）；
- 举报与审核队列、敏感词过滤；
- 班级登录（邀请码 / 学校 SSO）与更细的管理权限；
- 回复楼中楼、@ 提醒、每周精选；
- 用 CDN 托管 `/uploads`，Go 只负责 API。

---

## 9. 常用命令

```bash
make install    # 安装前端依赖
make dev-api    # 终端 A：Go 服务 :8080
make dev-web    # 终端 B：Astro :4321（代理 API）
make build      # 生产构建 → bin/shiguang-island
make run        # 构建并运行
make docker     # docker compose up -d --build
make clean      # 清理构建产物
```

---

## 10. 许可证

本项目基于 [MIT License](./LICENSE) 开源，© 2026 diyanqi。
