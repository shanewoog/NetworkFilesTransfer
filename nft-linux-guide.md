# NFT Linux 操作文档

本文档用于 Linux 服务器上的安装、更新、卸载和常用服务管理。

## 安装

1. 进入 `nft.sh` 所在目录

```bash
cd /path/to
```

2. 给脚本加执行权限

```bash
chmod +x nft.sh
```

3. 执行安装

```bash
sudo ./nft.sh install
```

默认行为:

- 安装目录: `/opt/nft`
- 服务名: `nft`
- 服务用户: `root`
- 防火墙: 自动按 `config.json` 中的 `port` 开放对应 TCP 端口
- 安装完成后不会自动启动服务

4. 修改配置后手动启动

```bash
sudo vi /opt/nft/config.json
sudo systemctl start nft
```

可选参数:

- 指定安装目录

```bash
sudo INSTALL_DIR=/data/nft ./nft.sh install
```

- 指定服务名

```bash
sudo SERVICE_NAME=nft ./nft.sh install
```

- 指定服务用户

```bash
sudo SERVICE_USER=www-data ./nft.sh install
```

## 更新

把新版本 `nft.sh` 上传到任意目录，在该目录执行:

```bash
sudo ./nft.sh update
```

更新脚本会:

- 停止当前服务
- 替换程序与静态文件
- 保留 `config.json`、`uploads/`、`temp/`、`share.db` 等运行数据
- 重新写入 systemd 服务文件并启动服务
- 完成后删除本次使用的 `nft.sh`

## 卸载

```bash
sudo /opt/nft/nft.sh uninstall
```

卸载脚本会:

- 停止并禁用 `nft` 服务
- 同时尝试清理旧的 `NetworkFilesTransfer` 服务
- 删除对应的 systemd 服务文件
- 询问是否删除安装目录（会一并删除配置和上传数据）

## 状态

```bash
sudo /opt/nft/nft.sh status
```

## 常用服务管理命令

```bash
systemctl start nft
systemctl stop nft
systemctl restart nft
systemctl status nft
journalctl -u nft -f
```