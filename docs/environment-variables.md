# Environment Variables Reference

This document provides a complete reference for all environment variables used by DeveloperAndQAAgent.

## Overview

The configuration system supports environment variable expansion using `${VAR_NAME}` syntax in YAML files. Variables are expanded at runtime when the configuration is loaded.

```yaml
github:
  token: "${GITHUB_TOKEN}"  # Expands to the value of GITHUB_TOKEN
```

## Required Variables

These environment variables must be set for the agent to function:

### `GITHUB_TOKEN`
- **Purpose**: Authentication for GitHub API access
- **Required Scopes**: `repo`, `read:user`
- **Format**: `ghp_xxxxxxxxxxxxxxxxxxxx` (classic token) or `github_pat_xxxxxxxxxxxxxxxxxxxx` (fine-grained token)
- **Generation**: [GitHub Settings > Developer Settings > Personal Access Tokens](https://github.com/settings/tokens)
- **Example**: `ghp_1234567890abcdef1234567890abcdef12345678`

**Security Notes**:
- Never commit this token to version control
- Use repository secrets in CI/CD environments
- Regenerate if compromised
- Consider using fine-grained tokens with minimal repository access

### `ANTHROPIC_API_KEY`
- **Purpose**: Authentication for Claude API access  
- **Format**: `sk-ant-api03-xxxxxxxxxxxx`
- **Generation**: [Anthropic Console](https://console.anthropic.com/)
- **Example**: `sk-ant-api03-1234567890abcdef1234567890abcdef1234567890abcdef`

**Security Notes**:
- Keep this key private and secure
- Monitor usage in Anthropic Console
- Rotate keys periodically

## Optional Variables

These variables can be used for additional configuration:

### `AGENTCTL_WORKSPACE_DIR`
- **Purpose**: Override workspace directory location
- **Default**: `./workspaces`
- **Usage**: `workspace_dir: "${AGENTCTL_WORKSPACE_DIR}"`
- **Example**: `/tmp/agent-workspaces`

### `AGENTCTL_STATE_DIR`
- **Purpose**: Override state storage directory
- **Default**: `.agentctl/state`
- **Usage**: `dir: "${AGENTCTL_STATE_DIR}"`
- **Example**: `/var/lib/agentctl/state`

### `AGENTCTL_LOG_LEVEL`
- **Purpose**: Override logging level
- **Valid Values**: `debug`, `info`, `warn`, `error`
- **Default**: `info`
- **Usage**: `level: "${AGENTCTL_LOG_LEVEL}"`

## Deployment-Specific Variables

### Docker Environment

```dockerfile
ENV GITHUB_TOKEN="ghp_your_token_here"
ENV ANTHROPIC_API_KEY="sk-ant-api03-your_key_here"
ENV AGENTCTL_WORKSPACE_DIR="/app/workspaces"
ENV AGENTCTL_STATE_DIR="/app/data/state"
```

### Kubernetes Environment

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: agentctl-secrets
type: Opaque
data:
  github-token: <base64-encoded-token>
  anthropic-api-key: <base64-encoded-key>
---
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: agentctl
        env:
        - name: GITHUB_TOKEN
          valueFrom:
            secretKeyRef:
              name: agentctl-secrets
              key: github-token
        - name: ANTHROPIC_API_KEY
          valueFrom:
            secretKeyRef:
              name: agentctl-secrets
              key: anthropic-api-key
```

### GitHub Actions Environment

```yaml
env:
  GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
  ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
```

## Validation and Troubleshooting

### Environment Variable Validation

Use the config validation command to check environment variable expansion:

```bash
agentctl validate --config configs/config.yaml --full
```

This will:
- ✅ Verify variables are set and not empty
- ✅ Check token format validity
- ✅ Test API connectivity (with `--full` flag)
- ❌ Report missing or invalid variables

### Common Issues

#### Variable Not Expanded
```
Error: github.token: required field is empty
```

**Causes**:
- Environment variable not set: `export GITHUB_TOKEN="your_token"`
- Typo in variable name: `${GITHUB_TOKN}` vs `${GITHUB_TOKEN}`
- Variable set but empty: `GITHUB_TOKEN=""`

#### Token Format Invalid
```
Error: github.token: token format appears invalid
```

**Causes**:
- Using wrong token type (OAuth app token vs personal access token)
- Token truncated or corrupted during copy/paste
- Using expired or revoked token

#### API Authentication Failed
```
Error: github.token: token authentication failed: 401 Unauthorized
```

**Causes**:
- Token expired or revoked
- Wrong token provided
- Insufficient scopes (missing `repo` scope)

### Security Best Practices

#### Development Environment
```bash
# Use environment-specific .env files (never commit)
echo "GITHUB_TOKEN=ghp_your_dev_token" > .env.local
echo "ANTHROPIC_API_KEY=sk-ant-your_dev_key" >> .env.local

# Load before running
source .env.local
agentctl start --config configs/config.yaml
```

#### Production Environment
```bash
# Use secure secret management
kubectl create secret generic agentctl-secrets \
  --from-literal=github-token="ghp_your_token" \
  --from-literal=anthropic-api-key="sk-ant-your_key"

# Or use external secret managers (AWS Secrets Manager, HashiCorp Vault, etc.)
```

#### Token Rotation
```bash
# Generate new token in GitHub/Anthropic console
# Update secret storage
# Restart agent with new configuration
# Verify connectivity
agentctl validate --config configs/config.yaml --full
```

## Debugging Environment Variable Issues

### Check Variable Values
```bash
# Check if variables are set (without exposing values)
echo ${GITHUB_TOKEN:+✅ GITHUB_TOKEN is set}
echo ${ANTHROPIC_API_KEY:+✅ ANTHROPIC_API_KEY is set}

# Check variable length (tokens should be 40+ chars)
echo "GitHub token length: ${#GITHUB_TOKEN}"
echo "Claude key length: ${#ANTHROPIC_API_KEY}"
```

### Test Configuration Loading
```bash
# Dry run configuration loading
agentctl validate --config configs/config.yaml

# Verbose validation with network checks
agentctl validate --config configs/config.yaml --full
```

### Environment Variable Expansion Test
```bash
# Create test config with debug values
cat > test-env.yaml << EOF
test_values:
  github_token: "${GITHUB_TOKEN}"
  claude_key: "${ANTHROPIC_API_KEY}"
  missing_var: "${NONEXISTENT_VAR}"
EOF

# Check expansion (be careful with secrets)
python3 -c "
import os
import yaml
with open('test-env.yaml') as f:
    data = yaml.safe_load(f)
    for key, value in data['test_values'].items():
        expanded = os.path.expandvars(value)
        print(f'{key}: {\"SET\" if expanded != value else \"NOT_SET\"}')
"
```

## Integration Examples

### CI/CD Pipeline Configuration

#### Jenkins
```groovy
pipeline {
    agent any
    environment {
        GITHUB_TOKEN = credentials('github-token')
        ANTHROPIC_API_KEY = credentials('anthropic-api-key')
    }
    stages {
        stage('Validate Config') {
            steps {
                sh 'agentctl validate --config configs/config.yaml'
            }
        }
    }
}
```

#### GitLab CI
```yaml
variables:
  GITHUB_TOKEN: $GITHUB_TOKEN
  ANTHROPIC_API_KEY: $ANTHROPIC_API_KEY

validate-config:
  script:
    - agentctl validate --config configs/config.yaml --full
```

### Docker Compose
```yaml
version: '3.8'
services:
  agentctl:
    image: agentctl:latest
    environment:
      - GITHUB_TOKEN=${GITHUB_TOKEN}
      - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}
      - AGENTCTL_WORKSPACE_DIR=/app/workspaces
      - AGENTCTL_STATE_DIR=/app/data/state
    volumes:
      - ./configs/config.yaml:/app/config.yaml
      - agent_workspaces:/app/workspaces
      - agent_state:/app/data/state
```

This comprehensive reference should help developers and operators correctly configure and troubleshoot environment variable usage in all deployment scenarios.