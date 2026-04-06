# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| latest  | ✅         |

During the alpha/beta phase, only the latest release receives security fixes.

## Reporting a Vulnerability

**Please do NOT report security vulnerabilities via GitHub Issues.**

Report vulnerabilities via [GitHub Private Security Advisories](https://github.com/tidefly-oss/tidefly-plane/security/advisories/new).

Include:
- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (optional)

### What to Expect

- **Acknowledgement** within 48 hours
- **Status update** within 7 days
- **Fix timeline** communicated as soon as assessed
- **Credit** in the release notes (if desired)

## Security Best Practices for Deployment

- Never expose Tidefly directly to the public internet without TLS
- Use strong, unique values for `APP_SECRET_KEY` (minimum 32 characters)
- Keep Docker socket access restricted — only Tidefly's backend process should have access
- Rotate secrets regularly via `task rotate-secrets`
- Enable HTTPS in production — Caddy with Let's Encrypt is built-in
- Review audit logs regularly via `GET /api/v1/logs/audit`

## Scope

**In scope:**
- Authentication and authorization bypasses
- SQL injection, XSS, CSRF
- Remote code execution
- Privilege escalation
- Sensitive data exposure
- Docker socket abuse / container escape vectors
- Webhook HMAC signature bypass
- mTLS / gRPC tunnel bypass between Plane and Agent

**Out of scope:**
- Vulnerabilities in self-hosted infrastructure (your own servers)
- Social engineering attacks
- Denial of service without demonstrated impact

---
<div align="center">
  <sub>Built with ❤️ by <a href="https://github.com/dbuettgen">@dbuettgen</a> · Part of the <a href="https://github.com/tidefly-oss">tidefly-oss</a> project</sub>
</div>