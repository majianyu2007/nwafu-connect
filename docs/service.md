# 系统服务

服务配置必须通过受限权限的 TOML 文件传递凭据，不要把密码或 TOTP 密钥写进命令行参数。

## Linux（systemd）

安装二进制和配置：

```bash
sudo install -m 0755 nwafu-connect /usr/local/bin/nwafu-connect
sudo install -d -m 0700 /etc/nwafu-connect
sudo install -m 0600 config.toml /etc/nwafu-connect/config.toml
```

创建 `/etc/systemd/system/nwafu-connect.service`：

```ini
[Unit]
Description=NWAFU Connect aTrust client
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/nwafu-connect -config /etc/nwafu-connect/config.toml
Restart=on-failure
RestartSec=5
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
```

启用并查看日志：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now nwafu-connect
sudo journalctl -u nwafu-connect -f
```

若启用 TUN 或自动路由，需要 root 或额外网络 capabilities；不要在未验证权限模型时添加 `User=`。

## macOS（launchd）

安装：

```bash
sudo install -m 0755 nwafu-connect /usr/local/bin/nwafu-connect
sudo install -d -m 0700 /usr/local/etc/nwafu-connect
sudo install -m 0600 config.toml /usr/local/etc/nwafu-connect/config.toml
sudo cp com.nwafu.connect.plist /Library/LaunchDaemons/com.nwafu.connect.plist
sudo chown root:wheel /Library/LaunchDaemons/com.nwafu.connect.plist
sudo launchctl bootstrap system /Library/LaunchDaemons/com.nwafu.connect.plist
```

仓库中的 `com.nwafu.connect.plist` 只引用配置文件，不内嵌任何凭据。日志写入 `/var/log/nwafu-connect.log` 和 `/var/log/nwafu-connect.err.log`。
