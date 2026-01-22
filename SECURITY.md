# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 1.x.x   | :white_check_mark: |
| < 1.0   | :x: (Pre-release)  |

> **Note**: This project is currently in pre-alpha stage. Security policies will be enforced starting from v1.0.0.

## Reporting a Vulnerability

We take security seriously. If you discover a security vulnerability, please report it responsibly.

### How to Report

**DO NOT** open a public GitHub issue for security vulnerabilities.

Instead, please report security vulnerabilities via email:

📧 **security@kv-shepherd.io** (or create a private security advisory on GitHub)

### What to Include

Please include the following information in your report:

1. **Description** of the vulnerability
2. **Steps to reproduce** the issue
3. **Affected versions**
4. **Potential impact** assessment
5. **Suggested fix** (if any)

### Response Timeline

| Stage | Timeline |
|-------|----------|
| Initial response | Within 48 hours |
| Severity assessment | Within 7 days |
| Fix development | Depends on severity |
| Public disclosure | After fix is released |

### Severity Levels

| Level | Description | Response |
|-------|-------------|----------|
| **Critical** | Remote code execution, data breach | Immediate patch release |
| **High** | Authentication bypass, privilege escalation | Patch within 7 days |
| **Medium** | Information disclosure, DoS | Patch in next release |
| **Low** | Minor issues | Addressed in roadmap |

## Security Measures

### Authentication & Authorization

- JWT-based authentication with configurable expiration
- Role-based access control (RBAC)
- Environment isolation (test/prod separation)

### Data Protection

- Kubeconfig encryption using AES-256-GCM
- Database credentials stored securely
- No sensitive data in logs

### Infrastructure

- PostgreSQL with TLS connections
- Kubernetes RBAC for service accounts
- Network policies (when deployed)

### Supply Chain Security

- Dependency scanning in CI
- Go module verification
- Container image signing (planned)

## Security Best Practices for Operators

### Deployment Recommendations

1. **Use TLS** for all external connections
2. **Rotate credentials** regularly
3. **Enable audit logging**
4. **Apply network policies** to restrict access
5. **Keep dependencies updated**

### PostgreSQL Configuration

```yaml
# Recommended security settings
ssl: on
ssl_cert_file: '/path/to/server.crt'
ssl_key_file: '/path/to/server.key'
password_encryption: scram-sha-256
```

### Kubernetes RBAC

Apply least-privilege RBAC for the service account:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubevirt-shepherd
rules:
  - apiGroups: ["kubevirt.io"]
    resources: ["virtualmachines", "virtualmachineinstances"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

## Acknowledgments

We appreciate responsible disclosure and will acknowledge security researchers who report valid vulnerabilities.

## Contact

For security-related questions that are not vulnerabilities:
- Open a [GitHub Discussion](https://github.com/kv-shepherd/shepherd/discussions) with the `security` label

---

*This security policy is inspired by industry best practices.*
