# NetworkFilesTransfer

`NetworkFilesTransfer` 是一个基于 Go、Gin 和 SQLite 的轻量文件传输服务。

它提供了开箱即用的上传页和下载页，支持：

- 大文件分片上传
- 秒传校验
- 分享码下载
- 下载次数限制
- 过期自动清理
- Cloudflare R2 后台同步
- Cloudflare R2 前端直传

适合这些场景：

- 局域网或内网文件传输
- 临时文件投递
- 自托管轻量文件分享
- 小团队素材交付

## 功能概览

- 前端按文件大小自动调整分片大小和并发数
- 秒传命中时直接复用已有分享链接
- 本地文件按哈希落盘，下载时显示原文件名
- 支持下载次数限制和过期清理
- 页面关闭时可主动取消上传
- 上传成功页支持复制链接、分享文案和二维码
- 启用 R2 后支持两种模式：
  - 先传服务器，再同步到 R2
  - 前端直接上传到 R2

## 技术栈

- 后端：Go + Gin
- 数据库：SQLite（`modernc.org/sqlite`）
- 前端：原生 HTML / JavaScript + 本地 CSS
- 构建脚本：PowerShell
- 部署方式：Linux + systemd

## 目录结构

```text
.
├── main.go
├── upload_mode.go
├── upload_sessions.go
├── r2_config.go
├── r2_cleanup.go
├── r2_replication.go
├── r2_multipart.go
├── r2_direct_upload.go
├── file_replicas.go
├── file_replica_uploads.go
├── index.html
├── download.html
├── app.css
├── qrcode.min.js
├── config.json
├── buildLinux64.ps1
├── install.sh
├── uninstall.sh
├── nft-linux-guide.md
├── version.txt
├── uploads/
├── temp/
├── share.db
└── dist/
```

## 上传模式

项目支持三种实际上传模式：

### 1. `local`

只上传到服务器本地。

触发条件：

- `r2.enabled = false`

### 2. `local_then_sync`

先上传到服务器本地，再由后台异步同步到 R2。

触发条件：

- `r2.enabled = true`
- 且 `upload_mode` 未配置
- 或 `upload_mode = "local_then_sync"`

### 3. `r2_direct`

前端直接把文件分片上传到 R2，服务端只负责：

- 秒传检查
- 上传会话管理
- 分片签名
- 完成合并
- 业务记录入库

触发条件：

- `r2.enabled = true`
- 且 `upload_mode = "r2_direct"`

## 当前业务流程

### 本地上传 / 本地后同步 R2

1. 前端选择文件
2. 读取文件前 10 MB，计算 SHA-256
3. 调用 `/api/upload/check`
4. 未命中时，分片上传到 `/api/upload/chunk`
5. 全部分片完成后调用 `/api/upload/merge`
6. 服务端合并文件并写入 `files`
7. 如启用 R2 且模式为 `local_then_sync`，后台异步上传到 R2

### 前端直传 R2

1. 前端选择文件
2. 读取文件前 10 MB，计算 SHA-256
3. 调用 `/api/upload/check`
4. 未命中时调用 `/api/r2/upload/init`
5. 前端按服务端返回的分片策略上传到 R2
6. 每个分片先调用 `/api/r2/upload/sign-part` 获取 presigned URL
7. 全部分片完成后调用 `/api/r2/upload/complete`
8. 服务端写入 `files` 和 `file_replicas`

## 环境要求

- Go 1.26 或更高版本
- Windows 或 Linux

## 本地运行

### 1. 安装依赖

```bash
go mod download
```

### 2. 配置参数

复制 `config.example.json` 为 `config.json` 后编辑本地配置：

```bash
cp config.example.json config.json
```

### 3. 启动服务

```bash
go run .
```

默认访问地址：

```text
http://127.0.0.1:9000
```

## 配置说明

示例：

```json
{
  "upload_mode": "local_then_sync",
  "port": "9000",
  "domain": "http://127.0.0.1:9000",
  "upload_dir": "./uploads",
  "temp_dir": "./temp",
  "db_path": "./share.db",
  "expire_hours": 24,
  "max_single_size_gb": 10,
  "max_total_size_gb": 20,
  "share_code_length": 6,
  "download_limit": 5,
  "retention_interval_min": 10,
  "r2": {
    "enabled": true,
    "endpoint": "https://<account>.r2.cloudflarestorage.com",
    "bucket": "nft-cdn",
    "access_key_id": "<access-key-id>",
    "secret_access_key": "<secret-access-key>",
    "region": "auto",
    "prefix": "nft/",
    "access_domain": "https://cdn.example.com"
  }
}
```

### 顶层配置

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `upload_mode` | `string` | 上传模式。可选 `local_then_sync`、`r2_direct`。当 `r2.enabled=false` 时实际总是 `local` |
| `port` | `string` | 服务监听端口 |
| `domain` | `string` | 站点访问域名，用于上传成功页和下载页链接展示 |
| `upload_dir` | `string` | 本地正式文件目录 |
| `temp_dir` | `string` | 本地分片临时目录 |
| `db_path` | `string` | SQLite 数据库文件路径 |
| `expire_hours` | `int` | 文件有效期，单位小时 |
| `max_single_size_gb` | `int64` | 单文件大小上限，单位 GB |
| `max_total_size_gb` | `int64` | 托管总容量上限，直传 R2 时同样生效 |
| `share_code_length` | `int` | 分享码长度 |
| `download_limit` | `int` | 单文件最大下载次数 |
| `retention_interval_min` | `int` | 后台清理任务执行间隔，单位分钟 |

### R2 配置

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `r2.enabled` | `bool` | 是否启用 R2 能力 |
| `r2.endpoint` | `string` | R2 S3 兼容 endpoint |
| `r2.bucket` | `string` | R2 bucket 名称 |
| `r2.access_key_id` | `string` | R2 Access Key ID |
| `r2.secret_access_key` | `string` | R2 Secret Access Key |
| `r2.region` | `string` | 通常为 `auto` |
| `r2.prefix` | `string` | 对象前缀 |
| `r2.access_domain` | `string` | 下载页在 R2 副本可用时返回的访问域名 |

## `upload_mode` 的默认规则

项目不是简单地“读配置值就完事”，实际规则如下：

- `r2.enabled = false`
  - 不管 `upload_mode` 是否填写，实际模式都是 `local`
- `r2.enabled = true` 且 `upload_mode` 缺失或为空
  - 实际模式默认是 `local_then_sync`
- `r2.enabled = true` 且 `upload_mode = "r2_direct"`
  - 才会启用前端直传 R2

## 过期与 R2 删除策略

当 R2 启用时，`expire_hours` 的处理规则如下：

- `expire_hours < 24`
  - 后端会在记录被删除时主动删除 R2 上已上传成功的对象
  - 适合希望早于 24 小时就真正移除远端对象的场景
- `expire_hours >= 24`
  - 保持当前逻辑
  - 如果 bucket 已经配置了 1 天生命周期规则，可以继续依赖该规则处理远端对象

## R2 直传注意事项

当使用 `upload_mode = "r2_direct"` 时，除了配置 `endpoint / bucket / key` 外，还必须给 R2 bucket 配置浏览器 CORS。

至少应允许：

- Origin：你的上传页域名
- Method：`PUT`
- Header：允许上传请求头
- Expose Header：`ETag`

本地开发时可参考：

```json
[
  {
    "AllowedOrigins": [
      "http://127.0.0.1:9000"
    ],
    "AllowedMethods": [
      "PUT",
      "GET",
      "HEAD"
    ],
    "AllowedHeaders": [
      "*"
    ],
    "ExposeHeaders": [
      "ETag"
    ],
    "MaxAgeSeconds": 3600
  }
]
```

## 前端分片策略

### 本地上传 / 本地后同步 R2

- 小于 200 MB：2 MB，1 并发
- 小于 1 GB：5 MB，2 并发
- 小于 5 GB：10 MB，3 并发
- 大于等于 5 GB：20 MB，3 并发

### 直传 R2

直传 R2 时，分片大小由服务端按 R2 multipart 规则动态决定，前端不再复用本地上传的分片策略。

## 主要接口

### 公共接口

- `GET /api/config`
- `GET /api/storage`
- `GET /api/file/:code`
- `GET /api/download/:code`
- `POST /api/upload/check`

### 本地上传接口

- `POST /api/upload/chunk`
- `POST /api/upload/merge`
- `POST /api/upload/cancel`

### R2 直传接口

- `POST /api/r2/upload/init`
- `POST /api/r2/upload/sign-part`
- `POST /api/r2/upload/complete`
- `POST /api/r2/upload/cancel`

## 数据表

### `files`

主文件记录，包含：

- `code`
- `name`
- `path`
- `size`
- `expire_at`
- `hash`
- `download_count`
- `download_limit`
- `primary_backend`

### `file_replicas`

文件副本状态表，记录 R2 同步状态。

### `file_replica_uploads`

R2 multipart 上传会话表。

### `file_replica_parts`

R2 multipart 已上传分片记录。

### `upload_sessions`

R2 前端直传上传会话表，不与 `file_replicas` 的“副本同步”语义混用。

## 构建

### 本地构建当前平台

```bash
go build -o NetworkFilesTransfer.exe .
```

### 构建 Linux amd64 发布包

```powershell
./buildLinux64.ps1
```

生成内容位于：

```text
dist/
```

典型发布文件包括：

- `nft`
- `config.json`
- `index.html`
- `download.html`
- `app.css`
- `qrcode.min.js`
- `favicon.svg`
- `install.sh`
- `uninstall.sh`
- `nft-linux-guide.md`

## Linux 部署

构建完成后，把 `dist/` 目录内容上传到 Linux 服务器，再执行：

```bash
chmod +x install.sh
sudo ./install.sh
```

也可以指定安装目录：

```bash
sudo INSTALL_DIR=/opt/nft ./install.sh
```

安装完成后可使用：

```bash
systemctl start nft
systemctl stop nft
systemctl restart nft
systemctl status nft
journalctl -u nft -f
```

卸载：

```bash
chmod +x uninstall.sh
sudo ./uninstall.sh
```

## Cloudflare 代理配置（免费 HTTPS）

如果你的域名托管在 Cloudflare，可以通过开启代理功能免费获得 HTTPS 访问。

### 前提条件

- 域名已托管在 Cloudflare
- 服务器已安装 nginx

### 配置步骤

#### 1. Cloudflare DNS 配置

添加 DNS 记录并开启代理（橙色云朵）：

| 类型 | 名称 | 内容 | 代理状态 |
|------|------|------|----------|
| A | example.com | 服务器 IP | 已代理（橙色云朵） |
| CNAME | www | example.com | 已代理（橙色云朵） |

#### 2. Cloudflare SSL 设置

进入 Cloudflare 控制台 → SSL/TLS → Overview：

- 选择 **Flexible** 模式（源站只有 HTTP）
- 如果源站已配置 SSL 证书，可选择 **Full** 或 **Full (Strict)**

#### 3. nginx 配置

创建 `/etc/nginx/conf.d/example.com.conf`：

```nginx
server {
    listen 80;
    server_name example.com www.example.com;
    client_max_body_size 10240m;
    add_header Strict-Transport-Security "max-age=31536000";

    location / {
        proxy_pass http://127.0.0.1:9000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # 大文件上传超时设置
        proxy_read_timeout 600s;
        proxy_send_timeout 600s;
        proxy_connect_timeout 600s;

        # 禁用缓冲
        proxy_request_buffering off;
        proxy_buffering off;
        client_body_buffer_size 128k;
    }
}
```

重载 nginx：

```bash
nginx -t && nginx -s reload
```

#### 4. 修改 config.json

将 `domain` 改为你的域名：

```json
{
    "port": "9000",
    "domain": "https://example.com"
}
```

#### 5. R2 CORS 配置（如使用 R2 直传）

如果启用了 R2 直传模式，需要在 R2 桶的 CORS 配置中添加你的域名：

```json
[
  {
    "AllowedOrigins": ["https://example.com", "https://www.example.com"],
    "AllowedMethods": ["GET", "PUT", "POST", "HEAD"],
    "AllowedHeaders": ["*"],
    "ExposeHeaders": ["ETag"],
    "MaxAgeSeconds": 3600
  }
]
```

### 访问流程

```
用户 --HTTPS--> Cloudflare --HTTP--> nginx:80 --HTTP--> Go应用:9000
```

Cloudflare 自动处理 SSL 证书和 HTTPS 访问，源站无需配置证书。

### 注意事项

- **Flexible 模式**：Cloudflare 到源站是 HTTP，适合内网或可接受的场景
- **Full 模式**：需要在源站配置 SSL 证书（可用 Cloudflare Origin Certificate）
- 修改域名后记得同步更新 R2 的 CORS 配置
- Cloudflare 免费版支持 SSL，无需额外付费

## 测试与检查

```bash
go test ./...
go vet ./...
```

## 开源建议

- 不要把真实的 R2 密钥提交到仓库
- 部署前确认 `config.json`、`version.txt`、`share.db` 是否需要纳入发布包
- 如果启用 `r2_direct`，先在 Cloudflare R2 配好 bucket CORS

## License

本项目使用 [MIT License](./LICENSE)。
