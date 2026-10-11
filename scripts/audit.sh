#!/usr/bin/env bash
# Usage: audit.sh. Scans the whole history for secrets and personal data, and
# the Go modules for known vulnerabilities.
set -euo pipefail

gitleaks git --config .gitleaks.toml --redact --no-banner --log-level warn .
govulncheck ./...
