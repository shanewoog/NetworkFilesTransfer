# 发布规则

本文档描述 `NetworkFilesTransfer` 当前实际执行的发布流程与约定。

## 版本号

- 唯一来源：仓库根目录 `version.txt`，格式为三段纯数字 `major.minor.patch`（如 `1.0.16`）。
- 当前固定前缀为 `1.0`（`main.go` 中 `defaultVersionPrefix`，`buildLinux64.ps1` 中 `$versionPrefix`）。
- 发布只自增 patch 位：`1.0.16` → `1.0.17`。若 `version.txt` 前缀不是 `1.0` 或格式非法，则重置为 `1.0.0`。
- 读取 `version.txt` 时会剥离 UTF-8 BOM。
- 运行时自动 bump（仅当可执行文件未被注入版本时）：可执行文件 mtime 晚于 `version.txt` 且路径不含 `/go-build`，则 patch+1 并回写 `version.txt`。正式发布以构建脚本注入的版本为准，不依赖该行为。

## 构建

仅构建 Linux amd64 发布包，在仓库根目录执行：

```powershell
./buildLinux64.ps1
```

脚本行为：

1. 读取 `version.txt`，计算下一个版本号。
2. 以 `GOOS=linux`、`GOARCH=amd64` 编译：`go build -ldflags "-X main.appVersion=<版本号>" -o nft .`
3. 编译成功后把新版本号写入根目录 `version.txt`。
4. 把发布文件（`nft`、`config.json`、`index.html`、`download.html`、`app.css`、`qrcode.min.js`、`favicon.svg`、`version.txt`、`nft-linux-guide.md`）打成 `tar.gz`，Base64 编码后追加到 `nft.sh` 末尾，输出**唯一**发布文件 `dist/nft.sh`。

`dist/` 内容只有一个文件：

```text
nft.sh
```

该文件同时是管理脚本和发布包：前半部分是 shell 脚本，末尾是 `__NFT_PAYLOAD_BELOW__` 标记行加 Base64 的 `tar.gz`。运行时脚本只读取标记行之后的数据解包到临时目录，安装/更新完成后临时目录自动删除。

## 管理脚本 nft.sh

`nft.sh` 提供四个子命令；不带子命令时会自动判断：安装目录不存在且服务未运行则执行 `install`，安装目录存在且服务正在运行则执行 `update`，其余情况打印用法并退出 1。

| 命令 | 行为 |
| --- | --- |
| `install` | 解包内嵌文件到 `/opt/nft`（可用 `INSTALL_DIR` 覆盖）；已存在的 `config.json` 会保留；写入 systemd 服务并 enable，但不启动服务 |
| `update` | 停止服务 → 替换程序与静态文件 → 保留 `config.json`、`uploads/`、`temp/`、`share.db` 等运行数据 → 重写服务文件并启动 → 删除本次执行的 `nft.sh` |
| `uninstall` | 停止并禁用服务，删除 systemd 服务文件，询问是否删除安装目录 |
| `status` | 输出 `systemctl status` 与配置路径 |
| 无参数 | 见上：自动 `install` / 自动 `update` / 打印用法 |

默认值：

- 安装目录：`/opt/nft`
- 服务名：`nft`
- 服务用户：`root`
- 端口：取 `config.json` 中的 `port`，缺省 `9000`，用于 firewalld 放行

可用环境变量覆盖：`INSTALL_DIR`、`SERVICE_NAME`、`SERVICE_USER`。

安装到 `/opt/nft` 后，目录内是：

```text
nft
config.json
index.html
download.html
app.css
qrcode.min.js
favicon.svg
version.txt
nft-linux-guide.md
nft.sh
.install-meta
```

安装到目录的 `nft.sh` 已去掉内嵌 payload（只剩脚本本体，约 10 KB），仍可用于 `uninstall` 和 `status`。`uploads/`、`temp/`、`share.db*` 由程序运行时自建，不由脚本复制。

## 打 tag

- tag 命名固定为 `v<版本号>`，与 `version.txt` 保持一致。
- 沿用现有约定，使用轻量 tag，打在提交 `version.txt` 更新的那个 commit 上。

## 发布步骤

1. 确认当前分支已合并待发布改动，工作区无无关改动。
2. 执行 `./buildLinux64.ps1`。
3. 校验：根目录 `version.txt` 等于本次预期版本号；`dist/` 下只有 `nft.sh`，且 `bash -n dist/nft.sh` 通过。
4. 提交版本号变更：`git add version.txt && git commit -m "Bump version to <版本号>"`。
5. 打 tag：`git tag v<版本号>`。
6. 推送：`git push origin main` 与 `git push origin v<版本号>`。
7. 上传 `dist/nft.sh` 到发布位置（如 GitHub Releases）。

不需要再单独打包 tar.gz：发布物就是 `dist/nft.sh`。

## 部署方式

服务器上只需上传一个 `nft.sh`，然后：

```bash
chmod +x nft.sh
sudo ./nft.sh install
sudo vi /opt/nft/config.json
sudo systemctl start nft
```

更新：把新版本 `nft.sh` 上传到任意目录，执行：

```bash
chmod +x nft.sh
sudo ./nft.sh update
```

更新会自动重启服务，并在完成后删除本次使用的 `nft.sh`。

卸载：`sudo /opt/nft/nft.sh uninstall`；状态：`sudo /opt/nft/nft.sh status`。
