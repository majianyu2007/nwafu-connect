# System services

Pass credentials through a permission-restricted TOML file. Never place the password or TOTP seed in command-line arguments.

## Linux (systemd)

Install the binary and configuration:

```bash
sudo install -m 0755 nwafu-connect /usr/local/bin/nwafu-connect
sudo install -d -m 0700 /etc/nwafu-connect
sudo install -m 0600 config.toml /etc/nwafu-connect/config.toml
```

Create `/etc/systemd/system/nwafu-connect.service`:

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

Enable it and inspect logs:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now nwafu-connect
sudo journalctl -u nwafu-connect -f
```

TUN mode and automatic routes require root or additional network capabilities. Do not add `User=` until that permission model has been verified.

## macOS (launchd)

Install the files:

```bash
sudo install -m 0755 nwafu-connect /usr/local/bin/nwafu-connect
sudo install -d -m 0700 /usr/local/etc/nwafu-connect
sudo install -m 0600 config.toml /usr/local/etc/nwafu-connect/config.toml
sudo cp com.nwafu.connect.plist /Library/LaunchDaemons/com.nwafu.connect.plist
sudo chown root:wheel /Library/LaunchDaemons/com.nwafu.connect.plist
sudo launchctl bootstrap system /Library/LaunchDaemons/com.nwafu.connect.plist
```

The repository's `com.nwafu.connect.plist` references the configuration file and embeds no credentials. Logs are written to `/var/log/nwafu-connect.log` and `/var/log/nwafu-connect.err.log`.
