# NFT Linux 操作文档

本文档用于 Linux 服务器上的安装、卸载和常用服务管理。

## 安装

1. 进入发布包所在目录

```bash
cd /path/to/dist
```

2. 给二进制和脚本加执行权限

```bash
chmod +x nft install.sh uninstall.sh
```

3. 执行安装

```bash
sudo ./install.sh
```

默认行为:

- 安装目录: 执行安装命令时的当前目录
- 服务名: `nft`
- 服务用户: `root`
- 防火墙: 自动按 `config.json` 中的 `port` 开放对应 TCP 端口

可选参数:

- 指定安装目录

```bash
sudo INSTALL_DIR=/opt/nft ./install.sh
```

- 指定服务名

```bash
sudo SERVICE_NAME=nft ./install.sh
```

## 卸载

在安装目录执行:

```bash
sudo ./uninstall.sh
```

卸载脚本会:

- 停止并禁用当前 `nft` 服务
- 同时尝试清理旧的 `NetworkFilesTransfer` 服务
- 删除对应的 systemd 服务文件
- 询问是否删除安装目录

## 常用服务管理命令

```bash
systemctl start nft
systemctl stop nft
systemctl restart nft
systemctl status nft
journalctl -u nft -f
```