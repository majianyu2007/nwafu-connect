# Docker deployment

The repository builds locally by default and is not tied to an upstream image registry:

```bash
cp config.toml.example config.toml
# Fill in the LDAP account, password, and totp_secret
docker compose up --build -d
```

`docker-compose.yml` builds and runs `nwafu-connect`, mounts `./config.toml` at `/home/nonroot/config.toml`, exposes SOCKS5 on `1080` and HTTP on `1081`, and restarts the container after an exit.

View logs:

```bash
docker compose logs -f nwafu-connect
```

Stop the deployment:

```bash
docker compose down
```

## Persisting the aTrust session

With this configuration:

```toml
client_data_file = "/home/nonroot/data/client_data.json"
```

add a persistent volume:

```yaml
services:
  nwafu-connect:
    volumes:
      - ./config.toml:/home/nonroot/config.toml:ro
      - nwafu-connect-data:/home/nonroot/data

volumes:
  nwafu-connect-data:
```

`config.toml` contains the password and TOTP seed; `client_data.json` contains session cookies. Never commit or share either file. Set the host configuration to mode `0600`.

`.github/workflows/docker.yml` only pushes when the repository variable `DOCKER_IMAGE` is configured, preventing accidental publication to the upstream project's namespace.
