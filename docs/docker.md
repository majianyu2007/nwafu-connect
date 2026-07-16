# Docker 部署

仓库默认本地构建，不绑定任何上游镜像仓库：

```bash
cp config.toml.example config.toml
# 填写 LDAP 账号、密码和 totp_secret
docker compose up --build -d
```

`docker-compose.yml` 会：

- 构建并运行 `nwafu-connect`；
- 挂载 `./config.toml` 到 `/home/nonroot/config.toml`；
- 暴露 SOCKS5 `1080` 和 HTTP `1081`；
- 在退出后自动重启。

查看日志：

```bash
docker compose logs -f nwafu-connect
```

停止：

```bash
docker compose down
```

## 保留 aTrust 会话

若配置了：

```toml
client_data_file = "/home/nonroot/data/client_data.json"
```

需要为容器增加持久卷：

```yaml
services:
  nwafu-connect:
    volumes:
      - ./config.toml:/home/nonroot/config.toml:ro
      - nwafu-connect-data:/home/nonroot/data

volumes:
  nwafu-connect-data:
```

`config.toml` 包含密码和 TOTP 密钥，`client_data.json` 包含会话 Cookie；两者都不得提交或分享。宿主机配置建议设置为 `chmod 600 config.toml`。

`.github/workflows/docker.yml` 仅在仓库变量 `DOCKER_IMAGE` 已配置时推送镜像，避免误发布到原项目命名空间。
