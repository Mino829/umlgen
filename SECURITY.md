# Security Policy

## Supported Versions

We release security updates for the latest minor version and the previous minor version of `umlgen`.

| Version | Supported          |
| ------- | ------------------ |
| 0.4.x   | :white_check_mark: |
| 0.3.x   | :white_check_mark: |
| < 0.3.0 | :x:                |

## Reporting a Vulnerability

Please report security vulnerabilities privately via GitHub Security Advisories:

[https://github.com/Mino829/umlgen/security/advisories/new](https://github.com/Mino829/umlgen/security/advisories/new)

Alternatively, you can email the maintainer directly. Contact information is available on the maintainer's GitHub profile.

Please include:
- A clear description of the vulnerability
- Steps to reproduce the issue
- The affected version(s)
- Any suggested remediation, if you have one

We aim to acknowledge reports within 72 hours and will provide updates on our investigation and remediation timeline.

## Security Design

`umlgen` is designed to run locally and does not send source code to external services by default. Source files are parsed locally, and generated PlantUML/SVG outputs are written to the local file system.

The GitHub Actions reusable workflow (`.github/workflows/umlgen-pr-diff.yml`) downloads a pre-built `umlgen` binary from this repository's releases and verifies its SHA-256 checksum before execution.

## Disclosure Policy

We follow a coordinated disclosure approach:
1. We investigate and develop a fix
2. We release a patched version
3. We publish a security advisory within 90 days of the report, or sooner if the vulnerability is actively exploited
