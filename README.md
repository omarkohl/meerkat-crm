# Meerkat CRM - a simple CRM for the personal life

<p align="center">
  <img src="docs/assets/meerkat-crm-logo.svg" alt="Meerkat CRM Logo" width="180" />
</p>

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Backend: Go](https://img.shields.io/badge/Backend-Go-00ADD8?logo=go)](https://golang.org)
[![Frontend: React](https://img.shields.io/badge/Frontend-React-61DAFB?logo=react)](https://reactjs.org)

## About the project

> [!NOTE]
>
> This is a fork of [fbuchner/meerkat-crm](https://github.com/fbuchner/meerkat-crm) and deviates
> from upstream. The Docker images under `ghcr.io/omarkohl` are built from this fork, not from
> upstream. The aim is to merge all changes upstream. If that does not work out, the fork may
> continue under a new name. For now it is only a fork with images in a different namespace. See
> [`DIFFERENCES.md`](DIFFERENCES.md) for what changed and [`FORK.md`](FORK.md) for branches and
> versions.

Meerkat CRM (Contact Relationship Management) is a self-hosted solution to keep track of your
important contacts. As your digital rolodex it reminds you of birthdays, helps you to keep in mind
dietary habits as well as names of spouses of contacts - and much more.

You can find the detailed documentation here:
[fbuchner.github.io/meerkat-crm/](https://fbuchner.github.io/meerkat-crm/). That site is published
by upstream. Documentation of things specific to this fork is in [`docs/`](docs/) and
[`FORK.md`](FORK.md) on GitHub and is not published as a site.

> [!TIP]
> **[Click here to try the Demo!](https://meerkat-crm-demo.fly.dev/login?username=demo&password=test_12345)**
> (user: demo, password: test_12345)
>
> Demo instance will be started on demand, expect some seconds delay. Demo data is reset
> periodically. Photo upload is disabled.

<p align="center">
  <img src="docs/assets/screengrab.gif" alt="Meerkat CRM Demo" />
</p>

## Features

- Contact management
  - add and search contacts
  - deleted contacts go to a trash and can be restored
  - group contacts by circles (e.g. friends, family, work)
  - store relationships of contacts (e.g. spouses, children)
  - CardDAV server for two-way synchronization with your phone's contact list and CardDAV client to
    use with your own external server
  - Network graph to visualize your contacts and relationships
- Notes and activities
  - social network style timeline for contacts
  - notes assigned to individual contacts
  - activities with one or multiple contacts
  - general notes (for e.g. journaling)
- Reminders
  - Keep in touch through reminders and get e-mail notifications (via SMTP or resend)
  - See upcoming birthdays
- Integrations
  - Import data (from CSV, VCF and Monica), export data to CSV and VCF
  - Connect external calendars
  - Subscribe to birthdays and reminders from your calendar app (ICS feed)
  - Webhooks to integrate with your own systems
  - SSO
- Operations
  - Built-in daily backups of the database and photos, downloadable by admins
- Usability
  - Multiple languages (currently EN, DE, IT, ES, FR)
  - Light and dark mode

## Installation

### Docker (Recommended)

Meerkat CRM ships as a single all-in-one image (`ghcr.io/omarkohl/meerkat-crm`) that bundles the
frontend and backend into one container. The easiest way to run it is with Docker Compose:

1. **Download the Docker Compose file:**

   ```sh
   curl -O https://raw.githubusercontent.com/omarkohl/meerkat-crm/omk/docker-compose.yml
   curl -O https://raw.githubusercontent.com/omarkohl/meerkat-crm/omk/.env.example
   ```

2. **Configure environment:**

   ```sh
   # Copy the environment template
   cp .env.example .env

   # Edit with your settings
   nano .env
   ```

3. **Start the container:**

   ```sh
   docker compose up -d
   ```

4. **Access the application:** Open http://localhost:7300 in your browser.

Prefer to skip Compose? You can run the image directly.

```sh
docker run -d --name meerkat --restart unless-stopped \
  -p 7300:8080 \
  -e JWT_SECRET_KEY='<paste a secret here, changing it will invalidate logins>' \
  -v "$(pwd)/data:/app/data" \
  -v "$(pwd)/photos:/app/static/photos" \
  ghcr.io/omarkohl/meerkat-crm:1.7.0-omk.1
```

All other settings are optional, see [`.env.example`](.env.example) for e-mail notifications,
CardDAV, and SSO. Pass them with additional `-e` flags, or use `--env-file .env`.

## Contributing

### Bugs and feature requests

This application is under development. You can
[open an issue](https://github.com/omarkohl/meerkat-crm/issues/new/choose) to report a bug or
request a new feature.

You can also participate and open up a pull request.

While AI-assistants can be used to support coding, please note that you are ultimately responsible
for code quality. Do not open pull requests for hands-off "vibe-coding" developments, rather stick
to feature requests in these cases.

### Development

You need [Go](https://golang.org/doc/install) and the Node.js version in `.node-version`.

```sh
git clone https://github.com/omarkohl/meerkat-crm.git
cd meerkat-crm
make dev        # backend and frontend with no setup; data goes to .dev/
make seed       # in a second terminal: a demo user and contacts
make precommit  # all checks, including E2E; run it before each commit
```

`make help` lists all targets. The project uses an SQLite database; migrations run on startup.

You can find a more comprehensive overview for developers in the [CONTRIBUTING.md](CONTRIBUTING.md).

## Alternative software

Notable other personal CRM systems are

- [MonicaHQ](https://www.monicahq.com/) (Open Source, development seems to have stalled; the new
  version chandler is available at beta.monicahq.com)
- [Dex](https://getdex.com/) (paid offering with social media integration)
- [Clay](https://clay.earth/) (paid offering with focus on automation)

Other software that can be used to build or configure something similar includes

- [Twenty](https://twenty.com/) (Open Source "classic" CRM system)
