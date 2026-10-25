# Contributing

## Bugs and feature requests

[Open an issue](https://github.com/omarkohl/meerkat-crm/issues/new/choose) to report a bug or
request a feature. Pull requests are welcome.

AI assistants are welcome, but you are responsible for the quality of what you submit and must
understand it well enough to explain it in review. Do not open hands-off "vibe-coded" pull requests
you have not read and tested; open a feature request instead.

## Set up

1. Fork and clone the repository. Fork changes live on the `omk` branch; open pull requests against
   `omk`.
2. Run `make dev`. It starts backend and frontend with no setup. Data goes to `.dev/`. Optional
   overrides go in `dev.env` (see `scripts/dev.sh`).
3. Run `make seed` to add a demo user (`demo` / `DemoPassword123!`) and contacts.
4. Run `make precommit` before each commit. `make help` lists all targets.

## Where to read more

The documentation sits in [`docs/`](docs/) and is shown on GitHub. The fork does not publish a docs
site. Upstream publishes the user documentation at
[fbuchner.github.io/meerkat-crm](https://fbuchner.github.io/meerkat-crm/).

- [Architecture](docs/development/architecture.md)
- [Backend](docs/development/backend.md)
- [Frontend](docs/development/frontend.md)
- [Testing](docs/development/testing.md)
- [Contributing and CI](docs/development/contributing.md)
- [Decisions (ADRs)](docs/adr/README.md)
- [Fork branches, versions and releases](docs/development/fork.md)
- [Rules for AI agents](AGENTS.md)
