# NetworkFilesTransfer

[简体中文](./README.md) | English

`NetworkFilesTransfer` is a lightweight self-hosted file transfer service built with Go, Gin, and SQLite.

Demo: https://5u.fit

It provides ready-to-use upload and download pages for temporary file sharing, LAN transfers, and small-team file delivery.

## Features

- Chunked upload for large files
- Fast duplicate detection by file hash
- Share-code based downloads
- Download count limits
- Automatic expiration and cleanup
- Local storage mode
- Cloudflare R2 background replication
- Cloudflare R2 direct browser upload
- QR code and share text after upload

## Tech Stack

- Backend: Go + Gin
- Database: SQLite (`modernc.org/sqlite`)
- Frontend: plain HTML, JavaScript, and CSS
- Optional storage: Cloudflare R2

## Quick Start

```bash
go mod download
cp config.example.json config.json
go run .
```

Open:

```text
http://127.0.0.1:9000
```

## Configuration

Edit `config.json` after copying it from `config.example.json`.

The default example uses local storage. To enable Cloudflare R2, set `r2.enabled` to `true` and configure the R2 endpoint, bucket, access key, secret key, and optional public access domain.

Supported upload modes:

- `local`: store files on the server
- `local_then_sync`: upload to the server first, then replicate to R2
- `r2_direct`: upload file chunks directly from the browser to R2 with server-side presigned URLs

## Test

```bash
go test ./...
go vet ./...
```

## License

This project is released under the [MIT License](./LICENSE).
