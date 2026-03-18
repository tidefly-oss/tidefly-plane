# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| latest  | ✅         |

During the alpha/beta phase, only the latest release receives security fixes.

## Reporting a Vulnerability

**Please do NOT report security vulnerabilities via GitHub Issues.**

Report vulnerabilities responsibly
via [GitHub Private Security Advisories](https://github.com/tidefly-oss/tidefly-backend/security/advisories/new).

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

- Never expose the Tidefly backend directly to the public internet without authentication
- Use strong, unique values for `APP_SECRET_KEY` (minimum 32 characters)
- Keep Docker/Podman socket access restricted — only Tidefly's backend process should have access
- Rotate secrets regularly via `task rotate-secrets`
- Enable TLS/HTTPS in production (Traefik integration built-in)
- Review and apply principle of least privilege for all service accounts

## Scope

**In scope:**

- Authentication and authorization bypasses
- SQL injection, XSS, CSRF
- Remote code execution
- Privilege escalation
- Sensitive data exposure
- Docker/Podman socket abuse / container escape vectors
- Webhook signature bypass

**Out of scope:**

- Vulnerabilities in self-hosted infrastructure (your own servers)
- Social engineering attacks
- Denial of service without demonstrated impact