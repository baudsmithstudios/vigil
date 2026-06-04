# Security Policy

Vigil is a small, self-hosted Raspberry Pi and homelab project. It is being developed and maintained on my spare time.

## Reporting a Vulnerability

Please do not open a public issue for security vulnerabilities.

Use GitHub's private vulnerability reporting from the repository [Security tab](https://github.com/baudsmithstudios/vigil/security) — click **"Report a vulnerability"** and describe the issue, its impact, and how to reproduce it. This creates a private advisory visible only to you and the maintainers.

## Scope

Reports are most useful when they describe the affected code. For example, the metrics collector, the service health checker, or the notification sender.

Vigil needs read-only host visibility (`/proc`, `/sys`, and the host PID and network namespaces) to collect accurate metrics, and the Docker socket is not mounted by default. Capability dropping and an optional socket proxy are documented in [Security](README.md#security). Granting Vigil more access than that guidance describes is a deployment choice, not a vulnerability.

General bugs, configuration questions, and documentation mistakes are best reported as regular issues.

## What to Expect

Response times vary. Confirmed issues are addressed as soon as is practical, and fixes land on the latest release. With your permission, you'll be credited in the advisory.
