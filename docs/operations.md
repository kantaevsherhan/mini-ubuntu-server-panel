# Эксплуатация

## Установка

```bash
curl -fsSL https://raw.githubusercontent.com/kantaevsherhan/mini-ubuntu-server-panel/main/scripts/install.sh | sudo bash
```

Установщик проверяет Ubuntu и архитектуру, скачивает Release, сверяет SHA-256, создаёт пользователя/директории, устанавливает systemd unit и проверяет health endpoint.

## Пути

- binary: `/opt/mini-ubuntu-server/bin/mini-ubuntu-server`;
- config: `/etc/mini-ubuntu-server/config.yml`;
- secrets: `/etc/mini-ubuntu-server/secrets.env`;
- database: `/var/lib/mini-ubuntu-server/mini-ubuntu-server.db`;
- backups: `/var/lib/mini-ubuntu-server/backups`;
- logs: `/var/log/mini-ubuntu-server`.

## Systemd

```bash
sudo systemctl status mini-ubuntu-server
sudo systemctl restart mini-ubuntu-server
sudo journalctl -u mini-ubuntu-server -f
```

## Release

```bash
scripts/release.sh v1.0.0
```

Скрипт проверяет frontend, собирает linux/amd64 и linux/arm64, создаёт архивы и `checksums.txt`. GitHub Actions выполняет тот же pipeline по tag `v*`.

## Backup и update

```bash
sudo scripts/update.sh v1.1.0
```

Update сохраняет предыдущий binary и SQLite, останавливает сервис, устанавливает новый binary, запускает сервис и проверяет health. При ошибке binary откатывается. Для production нужно дополнительно проверять SQLite integrity и откатывать несовместимые миграции.
