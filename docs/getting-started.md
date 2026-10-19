---
title: Getting Started
nav_order: 2
---

# Getting Started

## Installation

### Docker compose

Meerkat CRM ships as a single all-in-one image that bundles the frontend and backend into one
container, published in the github registry as `ghcr.io/fbuchner/meerkat-crm`. Images are available
for linux/amd64 and linux/arm64.

You can use either the `:latest` tag or a specific version (e.g. `:0.9` or `:0.9.1`).

Copy the
[sample docker compose file](https://github.com/fbuchner/meerkat-crm/blob/main/docker-compose.yml)
as well as [sample env file](https://github.com/fbuchner/meerkat-crm/blob/main/.env.example) and
rename the env file to `.env`.

After adjusting the environment variables as needed you can run: `docker compose up -d`

### Environment variables

| Variable                   | Description                                                                                                                                                                                                                          |
| -------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `JWT_SECRET_KEY`           | Random string used for JWT signing (minimum 32 characters)                                                                                                                                                                           |
| `FRONTEND_URL`             | Used for CORS headers. Wildcard (`*`) is allowed but not recommended for production use                                                                                                                                              |
| `RESEND_API_KEY`           | API key for [Resend](https://resend.com), used to send e-mail notifications. The generous free tier is more than enough for any personal setup                                                                                       |
| `RESEND_FROM_EMAIL`        | Sender e-mail address for Resend, needs to be configured in Resend                                                                                                                                                                   |
| `SMTP_HOST`                | SMTP server hostname, used to send e-mail notifications via your own mail server (alternative or in addition to Resend)                                                                                                              |
| `SMTP_PORT`                | SMTP server port (default `587`; use `465` with `SMTP_USE_TLS=true`)                                                                                                                                                                 |
| `SMTP_USERNAME`            | SMTP auth username (leave empty for unauthenticated relays)                                                                                                                                                                          |
| `SMTP_PASSWORD`            | SMTP auth password                                                                                                                                                                                                                   |
| `SMTP_FROM_EMAIL`          | Sender e-mail address for SMTP                                                                                                                                                                                                       |
| `SMTP_USE_TLS`             | Set to `true` for implicit TLS (port 465); otherwise STARTTLS is used                                                                                                                                                                |
| `CARDDAV_ENABLED`          | When set to `true` the application acts as a CardDAV server which allows contacts to be synced with your phone                                                                                                                       |
| `DISABLE_REGISTRATION`     | When set to `true`, new user registration is disabled (existing users can still log in). Default is `false`                                                                                                                          |
| `DATA_PATH`                | Host directory where the database file should be stored                                                                                                                                                                              |
| `PHOTOS_PATH`              | Host directory where the contact photos should be stored                                                                                                                                                                             |
| `JWT_EXPIRY_HOURS`         | Token expiry, i.e. after how many hours you will need to sign into the application again. Default is 96 hours (4 days)                                                                                                               |
| `OIDC_PROVIDER_URL`        | Issuer URL of your OIDC provider (i.e. endpoint URI for your provider). Required to enable SSO                                                                                                                                       |
| `OIDC_CLIENT_ID`           | OAuth2 client ID registered with your OIDC provider                                                                                                                                                                                  |
| `OIDC_CLIENT_SECRET`       | OAuth2 client secret registered with your OIDC provider                                                                                                                                                                              |
| `OIDC_AUTO_PROVISION`      | When `true`, a new account is automatically created on first SSO login. Default is `false`                                                                                                                                           |
| `OIDC_TRUST_EMAIL`         | When `true`, skips the `email_verified` requirement when linking an OIDC identity to an existing account by email. Safe to enable for self-hosted providers (e.g. Authentik) where you control all user accounts. Default is `false` |
| `REMINDER_TIME`            | Time of day at which reminder emails are sent, in `HH:MM` format (24-hour). Default is `12:00`                                                                                                                                       |
| `REMINDER_TIMEZONE`        | Timezone used for scheduling reminder emails. Must be a valid [IANA timezone name](https://en.wikipedia.org/wiki/List_of_tz_database_time_zones) (e.g. `Europe/Berlin`). Default is `UTC`                                            |
| `BACKUP_ENABLED`           | Whether the daily automatic [backup](deployment.md#backups) runs. Default is `true`. Admins can create backups by hand either way                                                                                                    |
| `BACKUP_TIME`              | Time of day of the automatic backup, `HH:MM` (24-hour) in `REMINDER_TIMEZONE`. Default is `02:30`                                                                                                                                    |
| `BACKUP_RETENTION_COUNT`   | Number of most recent backups to keep (older ones are deleted after each backup). Values below 1 fall back to the default, `7`                                                                                                       |
| `BACKUP_DIR`               | Directory the backups are written to. Default is a `backups` directory next to the database file (`SQLITE_DB_PATH`)                                                                                                                  |
| `AUDIT_LOG_RETENTION_DAYS` | Number of days [audit log](deployment.md#sessions-and-audit-log) entries are kept (purged daily). `0` keeps them forever. Default is `365`                                                                                           |
| `FAKE_NOW`                 | For demos and manual tests only: an RFC 3339 time such as `2026-12-24T12:00:00Z`. The server clock starts there and keeps running. Leave it unset on a real instance. See [ADR 0009](adr/0009-injected-clock.md)                     |
| `METRICS_ENABLED`          | Serve Prometheus metrics at `/metrics`. Default is `false`. See [Monitoring](deployment.md#monitoring-with-prometheus)                                                                                                               |
| `METRICS_TOKEN`            | Optional bearer token that scrapers must send to read the metrics                                                                                                                                                                    |
| `METRICS_ADDR`             | Optional separate listen address for the metrics (for example `127.0.0.1:9090`); then `/metrics` is not served on the main port                                                                                                      |
| `TRASH_RETENTION_DAYS`     | Number of days a deleted contact stays in the Trash before it is deleted permanently (checked daily). Values below 1 fall back to the default, `30`                                                                                  |
| `API_RATE_LIMIT_BURST`     | Requests one IP address may send in a burst to the API before it is slowed to 100 per minute. Default is `500`; raise it for scripts that send many requests                                                                         |

SSO is disabled unless all three of `OIDC_PROVIDER_URL`, `OIDC_CLIENT_ID`, and `OIDC_CLIENT_SECRET`
are set.

Other variables are found in the
[sample env file](https://github.com/fbuchner/meerkat-crm/blob/main/.env.example).

The backend process runs as a non-root user (default UID/GID 1001), and a startup script chowns the
data and photo directories to that user. Run `id` on your host to find your UID and GID and set them
as `PUID`/`PGID` in your `.env` file if you prefer folders to be owned by your host user (optional).

New photo directories are created with mode `0750` and photo files with `0640`, so other users on
the host cannot read them. Directories that already exist keep their mode (earlier versions created
them with `0777`); run `chmod 750 photos` on the host if you want to tighten them.

## Post-Installation Setup

When running Meerkat-CRM you can access the application under the specified port (default is
`7300`). To get started you need to register a user. The first user will automatically receive
administrator rights and therefore be able to access the admin panel in the settings menu.

## Backup

Meerkat makes a backup of the database and the photos every day and keeps the last 7 (see
[Backups](deployment.md#backups)). Copy them to a separate device regularly: they are stored on the
same disk as your data.
