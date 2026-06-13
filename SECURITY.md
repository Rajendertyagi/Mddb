# Security Policy

## Supported Versions

| Component | Version | Supported |
| --------- | ------- | --------- |
| Core server / CLI / panel (`vX.Y.Z` tags) | 2.x | :white_check_mark: |
| Core server / CLI / panel | 1.x | :x: |
| Integrations (own release tags: `wp-v*`, `gha-v*`, `chrome-ext-v*`, Airbyte/Grafana `0.x`) | latest `0.x` | :white_check_mark: |

The five `integrations/` packages (airbyte-destination, wordpress-plugin,
github-action, grafana-datasource, chrome-extension) ship as `0.x` under their
own release tags and are covered by this policy at their latest released version.

## Reporting a Vulnerability

We take the security of MDDB seriously. If you discover a security vulnerability, please report it responsibly.

### How to Report

1. **GitHub Security Advisories** (preferred): Go to [Security Advisories](https://github.com/tradik/mddb/security/advisories/new) and create a new advisory.
2. **Email**: Send details to **security@tradik.com**

### What to Include

- Description of the vulnerability
- Steps to reproduce
- Affected versions
- Potential impact
- Suggested fix (if any)

### Response Timeline

- **48 hours** - We will acknowledge receipt of your report
- **7 days** - We will provide an initial assessment
- **90 days** - We follow coordinated disclosure (we ask that you do not publicly disclose the vulnerability until a fix is available or 90 days have passed)

### Scope

The following components are in scope:

- `mddbd` - Database server (HTTP + gRPC, with the MCP server built in)
- `mddb-cli` - Command-line client
- `mddb-panel` - Web UI
- `mddb-chat` / `mddb-chat-widget` - Chat backend and embeddable widget
- `integrations/*` - Airbyte destination, WordPress plugin, GitHub Action,
  Grafana datasource, Chrome extension
- Client libraries / extensions under `clients/`, `services/php-extension`,
  `services/python-extension`

Out of scope:

- Third-party dependencies (please report to the respective maintainers)
- Issues in development/test configurations

### Recognition

We appreciate the security research community and will acknowledge reporters in our release notes (unless you prefer to remain anonymous).

## Security Best Practices

When deploying MDDB in production:

- Run the Docker container as a non-root user (default configuration)
- Use environment variables for sensitive configuration (API keys, embedding provider credentials)
- Keep MDDB updated to the latest supported version
- Restrict network access to the gRPC and HTTP ports
