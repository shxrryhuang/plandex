# Website

This website is built using [Docusaurus](https://docusaurus.io/), a modern static website generator.

### Installation

```
$ yarn
```

### Local Development

```
$ yarn start
```

This command starts a local development server and opens up a browser window. Most changes are reflected live without having to restart the server.

### Build

```
$ yarn build
```

This command generates static content into the `build` directory and can be served using any static contents hosting service.

### Deployment

Using SSH:

```
$ USE_SSH=true yarn deploy
```

Not using SSH:

```
$ GIT_USER=<Your GitHub username> yarn deploy
```

If you are using GitHub pages for hosting, this command is a convenient way to build the website and push to the `gh-pages` branch.

---

## Progress Reporting System Documentation

The progress reporting system provides clear, real-time visibility into CLI execution.

### Documentation Index

| Document | Description |
|----------|-------------|
| [progress-reporting.md](progress-reporting.md) | Main design documentation |
| [progress-reporting-feature-updates.md](progress-reporting-feature-updates.md) | Summary of all new features |
| [progress-pipeline.md](progress-pipeline.md) | Standalone test pipeline |
| [progress-reporting-bug-fixes.md](progress-reporting-bug-fixes.md) | Bug fixes in stream_tui |
| [progress-pipeline-error-fixes.md](progress-pipeline-error-fixes.md) | Error fixes in pipeline |

### Quick Start

```bash
cd app/cli

# Run standalone pipeline demo
go run ./progress/pipeline/cmd/

# Run tests with race detection
go test -race ./progress/pipeline/...
```
