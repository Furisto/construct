# Remote Daemon Setup Guide

This guide covers deploying and using Construct daemons remotely, enabling agent execution in isolated cloud environments, persistent task management, and multi-client access.

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Security Model](#security-model)
- [Quick Start](#quick-start)
- [Deployment Examples](#deployment-examples)
- [Authentication Setup](#authentication-setup)
- [Context Management](#context-management)
- [Advanced Configuration](#advanced-configuration)
- [Troubleshooting](#troubleshooting)
- [Best Practices](#best-practices)

---

## Overview

### What is Remote Daemon Mode?

Construct's daemon can run anywhere - on your local machine, a remote server, or in cloud sandboxes. The CLI connects to daemons via HTTP/2 (ConnectRPC), enabling distributed agent execution while maintaining the same familiar interface.

### Use Cases

**Isolated Execution**
Run agents in sandboxed environments separate from your development machine. Ideal for:
- Untrusted code execution
- Resource-intensive tasks
- Clean environment testing

**Persistent Agents**
Long-running tasks that continue even when you disconnect:
- Multi-day refactoring projects
- Continuous test runs
- Background analysis tasks

**Multi-Client Control**
Multiple CLI instances can interact with the same daemon:
- Team collaboration on shared tasks
- Switch between devices while maintaining context
- Monitor agent progress from different terminals

**Cloud Integration**
Deploy on platforms designed for isolated execution:
- Docker containers
- E2B sandboxes
- Fly.io instances
- Kubernetes clusters
- AWS Fargate

### Key Features

- **Token-based authentication**: Secure your remote daemon with cryptographically secure tokens
- **Setup codes**: Easy, secure bootstrapping without exposing tokens
- **Context switching**: Seamlessly switch between local and remote daemons
- **Transport flexibility**: Unix sockets for local, HTTPS for remote
- **Full API access**: All operations available locally work remotely

---

## Architecture

### Connection Model

```
┌─────────────────┐         HTTPS/HTTP/2         ┌─────────────────┐
│   CLI Client    │────────────────────────────>  │  Remote Daemon  │
│   (Your Laptop) │    Authorization: Bearer      │  (Cloud Server) │
└─────────────────┘         ct_abc123...          └─────────────────┘
                                                            │
                                                            ▼
                                                   ┌─────────────────┐
                                                   │  SQLite Database│
                                                   │  Agent State    │
                                                   └─────────────────┘
```

### Transport Types

**Unix Socket** (Local)
- Path: `/tmp/construct.sock` (macOS/Linux)
- Auth: Optional (OS permissions provide security)
- Use: Local development

**HTTP/2** (Remote)
- Protocol: HTTPS recommended (HTTP for internal networks)
- Auth: Required (Bearer tokens)
- Use: Remote/cloud deployments

### Authentication Flow

```
1. Admin creates setup code on server (via Unix socket)
   ↓
2. Setup code communicated to user securely (Slack, 1Password, etc.)
   ↓
3. User exchanges setup code for token (unauthenticated bootstrap)
   ↓
4. Token stored in system keyring (macOS Keychain, GNOME Keyring, etc.)
   ↓
5. All subsequent requests include token in Authorization header
```

---

## Security Model

### Threat Model

**What we protect against:**
- Unauthorized access to remote daemons
- Token theft from filesystem
- Replay attacks (via token expiration)
- Brute force attacks on setup codes (short expiry + single-use)

**What we assume:**
- Server-side daemon is not compromised
- Communication channel is encrypted (HTTPS)
- System keyring is secure
- Admin has physical/SSH access to server

### Token Security

**Generation**
- 32 bytes (256 bits) of cryptographic randomness via `crypto/rand`
- Prefix: `ct_` for easy identification in logs/configs
- Format: `ct_<base64url-encoded-32-bytes>`

**Storage**
- Server: Only SHA-256 hash stored in database
- Client: Plaintext stored in system keyring (encrypted by OS)
- Never logged or persisted in plaintext on server

**Expiration**
- Default: 90 days
- Maximum: 365 days
- Configurable per-token
- Enforced on every request

**Revocation**
- Immediate: Token deleted from database
- All subsequent requests fail
- No grace period

### Setup Code Security

**Design**
- 8 alphanumeric characters (excludes ambiguous: 0/O, 1/I)
- Format: `XXXX-XXXX` (hyphen for readability)
- Character set: `ABCDEFGHJKLMNPQRSTUVWXYZ23456789` (32 characters)
- Keyspace: 32^8 ≈ 1 trillion combinations

**Properties**
- Short-lived: 20 minute default, 72 hour maximum
- Single-use: Deleted immediately upon exchange
- Case-insensitive: User convenience
- In-memory only: Doesn't survive daemon restarts

**Brute Force Resistance**
- Rate limiting: Not implemented (rely on short expiry + network latency)
- With 20-minute window: Infeasible to try even 0.1% of keyspace
- Monitor: Check server logs for repeated failed exchanges

### Network Security

**HTTPS Configuration**

For production deployments, always use HTTPS:

```bash
# Generate self-signed certificate (development only)
openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -days 365 -nodes

# Run daemon with TLS
construct daemon run \
  --listen-http 0.0.0.0:8443 \
  --tls-cert cert.pem \
  --tls-key key.pem
```

**Recommendations**
- Use Let's Encrypt for production certificates
- Enable HSTS headers
- Use strong cipher suites (handled by Go's crypto/tls)
- Disable HTTP (HTTPS only)

**Firewall Rules**
```bash
# Only allow specific IPs to access daemon
ufw allow from 203.0.113.0/24 to any port 8443
```

### Unix Socket Security

For local development, Unix sockets provide security via OS permissions:

```bash
# Default socket location
/tmp/construct.sock

# Permissions (owner only)
srwx------ 1 user user 0 Jan  3 10:00 construct.sock
```

**No token required** - OS already verified process owner.

---

## Quick Start

### 5-Minute Setup

**1. Deploy daemon on remote server**

```bash
# SSH into your server
ssh user@server.example.com

# Start daemon listening on all interfaces
construct daemon run --listen-http 0.0.0.0:8080
```

**2. Create setup code**

```bash
# On the server (new terminal or SSH session)
construct daemon token create-setup my-laptop

# Output:
# ✅ Setup code created for token "my-laptop"
#
#   Code:    ABCD-1234
#   Expires: in 20 minutes
#
# Share this code securely with the user. They can exchange it for a token using:
#
#   construct context add <name> \
#     --endpoint <daemon-url> \
#     --setup-code ABCD-1234
#
# ⚠️  This code can only be used once and expires in 20 minutes.
```

**3. Connect from your laptop**

```bash
# On your local machine
construct context add production \
  --endpoint http://server.example.com:8080 \
  --setup-code ABCD-1234

# Output:
# ✅ Context "production" added successfully
# ✅ Token retrieved and stored in system keyring
```

**4. Start using the remote daemon**

```bash
# Switch to the remote context
construct context use production

# Start a new task (runs on remote daemon)
construct new --agent edit

# Or use one-off with --context flag
construct exec "analyze this codebase" --context production
```

Done! Your CLI is now connected to the remote daemon.

---

## Deployment Examples

### Docker

**Dockerfile**

```dockerfile
FROM golang:1.24-alpine AS builder

# Install dependencies
RUN apk add --no-cache git

# Build Construct
WORKDIR /build
COPY . .
RUN cd frontend/cli && go build -o construct

FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates sqlite

# Create construct user
RUN adduser -D -u 1000 construct

# Copy binary
COPY --from=builder /build/frontend/cli/construct /usr/local/bin/

# Set up data directory
RUN mkdir -p /data && chown construct:construct /data

USER construct
WORKDIR /data

# Expose daemon port
EXPOSE 8080

# Run daemon
ENTRYPOINT ["construct", "daemon", "run", "--listen-http", "0.0.0.0:8080", "--data-dir", "/data"]
```

**Build and run**

```bash
# Build image
docker build -t construct:latest .

# Run with persistent storage
docker run -d \
  --name construct-daemon \
  -p 8080:8080 \
  -v construct-data:/data \
  construct:latest

# Create setup code (exec into container)
docker exec construct-daemon construct daemon token create-setup laptop

# Or use Unix socket from host
docker exec construct-daemon construct daemon token create laptop --expires 90d
```

**Docker Compose**

```yaml
version: '3.8'

services:
  construct:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - construct-data:/data
    environment:
      - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}
    restart: unless-stopped

volumes:
  construct-data:
```

### Fly.io

Fly.io provides fast, distributed compute with built-in HTTPS.

**fly.toml**

```toml
app = "construct-daemon"
primary_region = "sea"

[build]
  dockerfile = "Dockerfile"

[env]
  DATA_DIR = "/data"

[http_service]
  internal_port = 8080
  force_https = true
  auto_stop_machines = true
  auto_start_machines = true
  min_machines_running = 0

[[vm]]
  cpu_kind = "shared"
  cpus = 1
  memory_mb = 1024

[[mounts]]
  source = "construct_data"
  destination = "/data"
```

**Deploy**

```bash
# Install flyctl
curl -L https://fly.io/install.sh | sh

# Login
fly auth login

# Create app and volume
fly apps create construct-daemon
fly volumes create construct_data --region sea --size 10

# Set secrets
fly secrets set ANTHROPIC_API_KEY=sk-ant-...

# Deploy
fly deploy

# Create setup code
fly ssh console -C "construct daemon token create-setup prod-access"

# Connect from laptop
construct context add fly-prod \
  --endpoint https://construct-daemon.fly.dev \
  --setup-code WXYZ-5678
```

**Benefits**
- Automatic HTTPS with valid certificates
- Global distribution (place close to users)
- Pay-per-use (auto-stop when idle)
- Built-in health checks and monitoring

### E2B (Code Interpreter SDK)

E2B provides secure, sandboxed cloud environments designed for code execution.

**Setup**

```python
# install_construct.py - Deploy Construct to E2B sandbox
from e2b_code_interpreter import CodeInterpreter

# Create sandbox
sandbox = CodeInterpreter()

# Upload Construct binary
sandbox.filesystem.write("/usr/local/bin/construct", construct_binary)
sandbox.process.start("chmod +x /usr/local/bin/construct")

# Start daemon
process = sandbox.process.start_and_wait(
    "construct daemon run --listen-http 0.0.0.0:8080",
    background=True
)

# Create token (via process exec)
result = sandbox.process.start_and_wait(
    "construct daemon token create e2b-client --output json"
)

token = json.loads(result.stdout)["token"]
print(f"Token: {token}")

# Connect from CLI
print(f"construct context add e2b --endpoint https://{sandbox.get_host()}:8080 --token {token}")

# Keep sandbox alive
input("Press Enter to destroy sandbox...")
sandbox.close()
```

**Use Case**: Untrusted code execution
- Full isolation from your infrastructure
- Automatic cleanup after execution
- Pre-configured with common tools
- API-driven provisioning

### AWS EC2

Traditional VPS deployment with full control.

**Launch instance**

```bash
# Ubuntu 24.04 LTS, t3.medium
aws ec2 run-instances \
  --image-id ami-0c55b159cbfafe1f0 \
  --instance-type t3.medium \
  --key-name my-key \
  --security-groups construct-daemon \
  --user-data file://install.sh

# Security group rules
aws ec2 authorize-security-group-ingress \
  --group-name construct-daemon \
  --protocol tcp \
  --port 8443 \
  --cidr 0.0.0.0/0
```

**install.sh**

```bash
#!/bin/bash
set -e

# Install Construct
curl -L https://github.com/furisto/construct/releases/latest/download/construct-linux-amd64 -o /usr/local/bin/construct
chmod +x /usr/local/bin/construct

# Create systemd service
cat > /etc/systemd/system/construct.service <<EOF
[Unit]
Description=Construct Daemon
After=network.target

[Service]
Type=simple
User=construct
Group=construct
WorkingDirectory=/var/lib/construct
Environment="ANTHROPIC_API_KEY=sk-ant-..."
ExecStart=/usr/local/bin/construct daemon run --listen-http 0.0.0.0:8443 --tls-cert /etc/construct/cert.pem --tls-key /etc/construct/key.pem
Restart=always

[Install]
WantedBy=multi-user.target
EOF

# Create user and directories
useradd -r -s /bin/false construct
mkdir -p /var/lib/construct /etc/construct
chown construct:construct /var/lib/construct

# Generate self-signed certificate (replace with Let's Encrypt in production)
openssl req -x509 -newkey rsa:4096 -keyout /etc/construct/key.pem -out /etc/construct/cert.pem -days 365 -nodes -subj "/CN=construct"

# Start service
systemctl enable construct
systemctl start construct
```

**Client setup**

```bash
# SSH into instance to create setup code
ssh -i my-key.pem ubuntu@ec2-203-0-113-25.compute-1.amazonaws.com
sudo construct daemon token create-setup my-laptop

# On laptop
construct context add aws-prod \
  --endpoint https://ec2-203-0-113-25.compute-1.amazonaws.com:8443 \
  --setup-code ABCD-1234
```

### Kubernetes

For large-scale deployments with orchestration.

**deployment.yaml**

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: construct

---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: construct-data
  namespace: construct
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi

---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: construct-daemon
  namespace: construct
spec:
  replicas: 1  # Single instance (SQLite limitation)
  selector:
    matchLabels:
      app: construct-daemon
  template:
    metadata:
      labels:
        app: construct-daemon
    spec:
      containers:
      - name: construct
        image: construct:latest
        ports:
        - containerPort: 8080
        env:
        - name: ANTHROPIC_API_KEY
          valueFrom:
            secretKeyRef:
              name: construct-secrets
              key: anthropic-api-key
        volumeMounts:
        - name: data
          mountPath: /data
        resources:
          requests:
            memory: "512Mi"
            cpu: "500m"
          limits:
            memory: "2Gi"
            cpu: "2000m"
      volumes:
      - name: data
        persistentVolumeClaim:
          claimName: construct-data

---
apiVersion: v1
kind: Service
metadata:
  name: construct-daemon
  namespace: construct
spec:
  selector:
    app: construct-daemon
  ports:
  - port: 8080
    targetPort: 8080
  type: LoadBalancer

---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: construct-ingress
  namespace: construct
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  tls:
  - hosts:
    - construct.example.com
    secretName: construct-tls
  rules:
  - host: construct.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: construct-daemon
            port:
              number: 8080
```

**Deploy**

```bash
# Create namespace and secret
kubectl create namespace construct
kubectl create secret generic construct-secrets \
  --namespace construct \
  --from-literal=anthropic-api-key=sk-ant-...

# Deploy
kubectl apply -f deployment.yaml

# Wait for LoadBalancer IP
kubectl get svc -n construct construct-daemon

# Create setup code
kubectl exec -n construct deployment/construct-daemon -- construct daemon token create-setup k8s-access

# Connect from laptop
construct context add k8s-prod \
  --endpoint https://construct.example.com \
  --setup-code VWXY-9012
```

---

## Authentication Setup

### Creating Tokens

**Method 1: Direct token creation** (SSH access required)

```bash
# SSH into server
ssh user@server.example.com

# Create token with default 90-day expiry
construct daemon token create my-laptop

# Output:
# ✅ Token created successfully
#
#   Name:    my-laptop
#   Token:   ct_abc123xyz...
#   Expires: 2025-06-15 14:30:00 UTC
#
# ⚠️  Important: Save this token securely - it cannot be retrieved again.
#     This token grants full access to the daemon.

# Create token with custom expiry and description
construct daemon token create ci-pipeline \
  --description "GitHub Actions pipeline" \
  --expires 30d

# JSON output for scripting
construct daemon token create automation \
  --output json | jq -r '.token'
```

**Method 2: Setup codes** (recommended)

```bash
# Server: Create setup code
construct daemon token create-setup laptop-token

# Share code securely (Slack, 1Password, verbally)
# Code: ABCD-1234

# Client: Exchange for token
construct context add prod \
  --endpoint https://daemon.example.com:8080 \
  --setup-code ABCD-1234
```

### Managing Tokens

**List all tokens**

```bash
# Show active tokens
construct daemon token list

# Output:
# ID                                    NAME             CREATED      EXPIRES      STATUS
# b7f8c9d0-1234-5678-90ab-cdef12345678  laptop-thomas    2 days ago   in 88 days   Active
# a1b2c3d4-5678-90ab-cdef-123456789012  ci-pipeline      1 week ago   in 23 days   Active

# Include expired tokens
construct daemon token list --include-expired

# Filter by name prefix
construct daemon token list --name-prefix prod

# JSON output
construct daemon token list --output json
```

**Revoke a token**

```bash
# Revoke with confirmation prompt
construct daemon token revoke b7f8c9d0-1234-5678-90ab-cdef12345678

# Force revoke (no prompt)
construct daemon token revoke a1b2c3d4-5678-90ab-cdef-123456789012 --force
```

### Token Rotation

For long-running deployments, rotate tokens periodically:

```bash
# Create new token
NEW_TOKEN=$(construct daemon token create laptop-new --expires 90d --output json | jq -r '.token')

# Update context with new token
construct context remove laptop --force
construct context add laptop \
  --endpoint https://daemon.example.com:8080 \
  --auth-token

# When prompted, paste the new token

# Revoke old token
construct daemon token revoke <old-token-id> --force
```

---

## Context Management

### Understanding Contexts

A context defines:
- **Endpoint**: Where the daemon is (Unix socket or HTTPS URL)
- **Authentication**: Token for remote daemons (stored in system keyring)
- **Name**: Human-readable identifier

### Context Commands

**Add a new context**

```bash
# Remote daemon with setup code (recommended)
construct context add staging \
  --endpoint https://staging.example.com:8443 \
  --setup-code WXYZ-5678

# Remote daemon with token prompt
construct context add production \
  --endpoint https://prod.example.com:8443 \
  --auth-token
# You'll be prompted to enter the token securely

# Local daemon (no auth)
construct context add local \
  --endpoint unix:///tmp/construct.sock

# Make current immediately
construct context add dev \
  --endpoint https://dev.example.com:8080 \
  --setup-code QRST-3456 \
  --set-current
```

**List contexts**

```bash
# Show all contexts
construct context list

# Output:
# NAME        ENDPOINT                                 AUTH    CURRENT
# local       unix:///tmp/construct.sock              No      *
# staging     https://staging.example.com:8443        Yes
# production  https://prod.example.com:8443           Yes

# Details view
construct context list --output yaml
```

**Switch contexts**

```bash
# Switch to a specific context
construct context use production

# Switch back to previous context
construct context use -

# Check current context
construct context current
```

**View context details**

```bash
# Show specific context
construct context show production

# Output:
# Name:     production
# Endpoint: https://prod.example.com:8443
# Kind:     http
# Auth:     Configured (keyring://construct/production)
```

**Remove a context**

```bash
# Remove with confirmation
construct context remove staging

# Force remove (no prompt)
construct context remove old-context --force
```

### Context Resolution

When you run a command, Construct determines which context to use:

**Priority order:**
1. `--context` flag
2. `CONSTRUCT_CONTEXT` environment variable
3. Current context from `~/.construct/contexts.yaml`

**Examples:**

```bash
# Use current context (from config)
construct new --agent edit

# Override with flag (one-time)
construct exec "analyze code" --context production

# Override with environment variable
export CONSTRUCT_CONTEXT=staging
construct new --agent plan

# Explicitly use local (even if remote is current)
construct task list --context local
```

### Configuration File

Contexts are stored in `~/.construct/contexts.yaml`:

```yaml
current: production
previous: local  # Used by "construct context use -"

contexts:
  local:
    endpoint: unix:///tmp/construct.sock
    # No auth for Unix sockets

  production:
    endpoint: https://prod.example.com:8443
    auth:
      type: token
      token-ref: keyring://construct/production  # Token stored in OS keyring

  staging:
    endpoint: https://staging.example.com:8443
    auth:
      type: token
      token-ref: keyring://construct/staging
```

**Never commit this file to version control** - it may reference sensitive tokens.

### Keyring Storage

Tokens are stored securely in your OS keyring:

- **macOS**: Keychain
- **Linux**: GNOME Keyring, KWallet (via Secret Service API)
- **Windows**: Credential Manager

View/manage tokens using OS tools:

```bash
# macOS: Open Keychain Access.app
# Search for "construct"

# Linux (GNOME)
seahorse  # GUI
secret-tool lookup service construct account production  # CLI

# Windows
# Control Panel → Credential Manager → Windows Credentials
```

---

## Advanced Configuration

### Daemon Options

**Listen addresses**

```bash
# Listen on all interfaces (0.0.0.0)
construct daemon run --listen-http 0.0.0.0:8080

# Listen on specific interface
construct daemon run --listen-http 192.168.1.100:8080

# Listen on both Unix socket and HTTP
construct daemon run \
  --listen-unix /tmp/construct.sock \
  --listen-http 0.0.0.0:8080
```

**Data directory**

```bash
# Custom data directory
construct daemon run \
  --listen-http 0.0.0.0:8080 \
  --data-dir /var/lib/construct

# Data directory contents:
# - construct.db (SQLite database)
# - logs/ (daemon logs)
```

**TLS configuration**

```bash
# With TLS certificates
construct daemon run \
  --listen-http 0.0.0.0:8443 \
  --tls-cert /etc/construct/cert.pem \
  --tls-key /etc/construct/key.pem

# Generate self-signed (development only)
openssl req -x509 -newkey rsa:4096 \
  -keyout key.pem -out cert.pem \
  -days 365 -nodes \
  -subj "/CN=localhost"
```

### Environment Variables

**Daemon configuration**

```bash
# API keys for model providers
export ANTHROPIC_API_KEY=sk-ant-...
export OPENAI_API_KEY=sk-...

# Override data directory
export CONSTRUCT_DATA_DIR=/var/lib/construct

# Log level
export CONSTRUCT_LOG_LEVEL=debug  # debug, info, warn, error
```

**Client configuration**

```bash
# Override current context
export CONSTRUCT_CONTEXT=production

# Override contexts file location
export CONSTRUCT_CONTEXTS_FILE=~/.config/construct/contexts.yaml
```

### Systemd Service (Linux)

For production deployments on Linux servers.

**/etc/systemd/system/construct.service**

```ini
[Unit]
Description=Construct AI Daemon
After=network.target

[Service]
Type=simple
User=construct
Group=construct
WorkingDirectory=/var/lib/construct

# Environment
Environment="ANTHROPIC_API_KEY=sk-ant-..."
Environment="CONSTRUCT_DATA_DIR=/var/lib/construct"
Environment="CONSTRUCT_LOG_LEVEL=info"

# Command
ExecStart=/usr/local/bin/construct daemon run \
  --listen-http 0.0.0.0:8443 \
  --tls-cert /etc/construct/cert.pem \
  --tls-key /etc/construct/key.pem

# Security
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/construct

# Restart policy
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

**Manage service**

```bash
# Enable and start
sudo systemctl enable construct
sudo systemctl start construct

# Status and logs
sudo systemctl status construct
sudo journalctl -u construct -f

# Restart
sudo systemctl restart construct
```

### Reverse Proxy (Nginx)

Use Nginx for HTTPS termination and load balancing.

**/etc/nginx/sites-available/construct**

```nginx
upstream construct_backend {
    server 127.0.0.1:8080;
}

server {
    listen 443 ssl http2;
    server_name construct.example.com;

    # SSL configuration
    ssl_certificate /etc/letsencrypt/live/construct.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/construct.example.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;

    # Proxy settings for ConnectRPC (HTTP/2)
    location / {
        grpc_pass grpc://construct_backend;
        grpc_set_header Host $host;
        grpc_set_header X-Real-IP $remote_addr;
        grpc_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        grpc_set_header X-Forwarded-Proto $scheme;
    }

    # Rate limiting (optional)
    limit_req_zone $binary_remote_addr zone=construct:10m rate=10r/s;
    limit_req zone=construct burst=20;
}

# Redirect HTTP to HTTPS
server {
    listen 80;
    server_name construct.example.com;
    return 301 https://$server_name$request_uri;
}
```

Enable and restart:

```bash
sudo ln -s /etc/nginx/sites-available/construct /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl restart nginx
```

### Monitoring and Logging

**Structured logging**

Construct daemon outputs structured JSON logs:

```json
{"level":"info","time":"2026-01-03T10:30:00Z","msg":"Daemon started","listen_http":"0.0.0.0:8080"}
{"level":"info","time":"2026-01-03T10:30:15Z","msg":"Request authenticated","subject":"laptop-thomas","method":"token"}
{"level":"warn","time":"2026-01-03T10:30:20Z","msg":"Token validation failed","token_hash":"abc123...","reason":"expired"}
```

**Prometheus metrics** (Coming soon)

Future support for metrics exposition:
- Request rate and latency
- Token validation success/failure
- Active tasks and agents
- Model provider API calls

---

## Troubleshooting

### Connection Issues

**Problem: "connection refused"**

```
Error: failed to connect to daemon: connection refused
```

Solutions:
- Verify daemon is running: `ps aux | grep construct`
- Check listen address: `--listen-http 0.0.0.0:8080` (not `127.0.0.1`)
- Verify firewall rules: `ufw status` or `iptables -L`
- Test with curl: `curl http://server:8080/`

**Problem: "context deadline exceeded"**

```
Error: context deadline exceeded
```

Solutions:
- Network latency too high (increase timeout)
- Daemon overloaded (check CPU/memory)
- Firewall blocking connection

### Authentication Issues

**Problem: "invalid or expired token"**

```
Error: rpc error: code = PermissionDenied desc = invalid or expired token
```

Solutions:
- Check token expiration: `construct daemon token list`
- Verify token in keyring: `secret-tool lookup service construct account production`
- Recreate token and update context
- Check server time synchronization (NTP)

**Problem: "setup code invalid or expired"**

```
Error: rpc error: code = PermissionDenied desc = invalid or expired setup code
```

Solutions:
- Setup codes expire quickly (20 minutes default)
- Setup codes are single-use (already consumed?)
- Check for typos (case-insensitive but hyphen matters)
- Generate new setup code

**Problem: "token management requires local daemon access"**

```
Error: token management requires local daemon access via Unix socket
```

Explanation:
- Token creation/management is admin-only
- Must connect via Unix socket (local access)
- Cannot create tokens via remote HTTP connection

Solution:
- SSH into server
- Run command locally (will use Unix socket)

### Context Issues

**Problem: "context not found"**

```
Error: context "production" not found
```

Solutions:
- List contexts: `construct context list`
- Add context: `construct context add production ...`
- Check contexts file: `cat ~/.construct/contexts.yaml`

**Problem: Keyring access denied**

```
Error: failed to retrieve token from keyring: access denied
```

Solutions:
- **macOS**: Grant terminal app access in System Preferences → Security → Privacy → Keychain
- **Linux**: Ensure gnome-keyring or kwallet is running
- **All**: Re-add context (will prompt for token again)

### Performance Issues

**Problem: High latency on remote daemon**

Symptoms:
- Slow command responses
- Timeouts on long operations

Solutions:
- Use closer geographic region
- Increase instance resources (CPU/memory)
- Check network bandwidth between client and server
- Consider local daemon for low-latency work

**Problem: Daemon high memory usage**

Symptoms:
- OOM kills in Docker/Kubernetes
- Slow responses

Solutions:
- Increase memory limits
- Clean up old tasks: `construct task list` and manually delete
- Restart daemon periodically
- Monitor with `top` or `htop`

### Debugging

**Enable debug logging (daemon)**

```bash
construct daemon run \
  --listen-http 0.0.0.0:8080 \
  --log-level debug
```

**Enable debug logging (client)**

```bash
export CONSTRUCT_LOG_LEVEL=debug
construct new --agent edit
```

**Inspect contexts file**

```bash
cat ~/.construct/contexts.yaml
```

**Test connectivity**

```bash
# Without auth
curl http://server:8080/

# With auth
curl -H "Authorization: Bearer ct_abc123..." \
  http://server:8080/construct.v1.AgentService/ListAgents
```

**Check keyring contents**

```bash
# macOS
security find-generic-password -s construct -a production

# Linux (GNOME)
secret-tool search service construct

# Windows PowerShell
cmdkey /list | findstr construct
```

---

## Best Practices

### Security

**1. Always use HTTPS for remote daemons**

```bash
# ✅ Good
--endpoint https://daemon.example.com:8443

# ❌ Bad (unencrypted)
--endpoint http://daemon.example.com:8080
```

**2. Use setup codes instead of sharing tokens**

```bash
# ✅ Good: Setup code (short-lived, single-use)
construct daemon token create-setup laptop
# Share code: ABCD-1234

# ❌ Bad: Direct token sharing
construct daemon token create laptop --output json
# Copying token to Slack/email exposes it
```

**3. Set appropriate token expiration**

```bash
# Short-lived for personal devices
construct daemon token create laptop --expires 30d

# Longer for CI/CD
construct daemon token create ci-pipeline --expires 180d

# Never set to maximum without reason
# ❌ Bad: construct daemon token create ... --expires 365d
```

**4. Rotate tokens periodically**

```bash
# Every 90 days
0 0 1 */3 * /usr/local/bin/rotate-construct-token.sh
```

**5. Revoke tokens immediately on compromise**

```bash
# If a device is lost/stolen
construct daemon token revoke <token-id> --force
```

**6. Use firewall rules to restrict access**

```bash
# Only allow specific IPs
ufw allow from 203.0.113.0/24 to any port 8443

# Or use VPN for access
```

### Operational

**1. Use systemd for daemon management**

- Automatic restarts on failure
- Resource limits
- Logging with journald

**2. Monitor disk usage**

SQLite database grows over time:

```bash
# Check database size
du -h /var/lib/construct/construct.db

# Archive old data (future feature)
construct task archive --before 2025-01-01
```

**3. Backup database regularly**

```bash
# Stop daemon
systemctl stop construct

# Backup database
cp /var/lib/construct/construct.db /backup/construct-$(date +%Y%m%d).db

# Restart daemon
systemctl start construct
```

**4. Set resource limits**

```bash
# Docker
docker run --memory=2g --cpus=2 construct:latest

# Kubernetes (see deployment.yaml above)
resources:
  limits:
    memory: "2Gi"
    cpu: "2000m"
```

**5. Use contexts consistently**

```bash
# Name contexts by environment
construct context add prod-us-east ...
construct context add staging-eu-west ...

# Not by team/person (use tokens for that)
# ❌ Bad: construct context add johns-daemon ...
```

### Development Workflow

**1. Use local daemon for development**

```bash
# Fast, no network latency
construct context use local
construct new --agent edit
```

**2. Use remote daemon for CI/CD**

```bash
# In GitHub Actions
- name: Run tests with Construct
  env:
    CONSTRUCT_CONTEXT: ci-daemon
  run: |
    construct exec "run full test suite" --wait
```

**3. Use remote daemon for isolation**

```bash
# Testing untrusted code
construct context use sandbox
construct exec "analyze this npm package for vulnerabilities"
```

**4. Multiple contexts for multiple environments**

```bash
# Development
construct context use dev

# Staging (before production)
construct context use staging
construct exec "validate migration scripts"

# Production
construct context use prod
```

---

## FAQ

**Q: Can multiple users share the same daemon?**

A: Yes. Each user creates their own token and context pointing to the shared daemon. All users can see all tasks (multi-tenancy is a future feature).

**Q: What happens if the daemon crashes mid-task?**

A: The task state is persisted in SQLite. When the daemon restarts, you can resume the task with `construct resume <task-id>`. However, any in-flight API calls to model providers will be lost.

**Q: Can I run multiple daemons and load balance?**

A: Not currently. SQLite database is local to each daemon. Future versions may support multi-daemon deployments with shared database.

**Q: Do tokens survive daemon restarts?**

A: Yes, tokens are stored in the SQLite database. Setup codes are in-memory only and will be lost.

**Q: How do I update Construct on a remote daemon?**

A: Replace the binary and restart:

```bash
# Download new version
curl -L https://github.com/furisto/construct/releases/download/v0.2.0/construct-linux-amd64 -o construct
sudo mv construct /usr/local/bin/

# Restart daemon
sudo systemctl restart construct
```

**Q: Can I use the same token on multiple machines?**

A: Yes, tokens are not tied to a specific client. You can use the same token on multiple devices. However, for security, we recommend separate tokens per device for easier revocation.

**Q: What's the performance impact of remote vs local daemon?**

A: Network latency adds overhead to each RPC call (typically 10-100ms depending on distance). For interactive sessions, this is usually acceptable. For high-frequency operations, local daemon is faster.

**Q: Can I use Construct without internet access?**

A: The daemon requires internet to call model provider APIs (Anthropic, OpenAI). The CLI-to-daemon connection works offline if both are local or on the same network.

---

## Additional Resources

- [CLI Reference](cli_reference.md) - Complete command documentation
- [Architecture Documentation](architecture.md) - Deep dive into Construct's design
- [API Reference](https://docs.construct.sh/api) (Coming soon)
- [GitHub Issues](https://github.com/furisto/construct/issues) - Report bugs and request features

---

**Last updated**: 2026-01-03
**Construct version**: 0.2.0+
