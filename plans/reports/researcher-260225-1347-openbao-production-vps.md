# OpenBao Production VPS Setup — Research Report

Date: 2026-02-25

---

## 1. Installation Methods

### Recommended: Package Manager (deb/rpm)

```bash
# Ubuntu/Debian
curl -fsSL https://openbao.org/repo/gpg | sudo gpg --dearmor -o /usr/share/keyrings/openbao.gpg
echo "deb [signed-by=/usr/share/keyrings/openbao.gpg] https://openbao.org/repo/apt stable main" \
  | sudo tee /etc/apt/sources.list.d/openbao.list
sudo apt update && sudo apt install openbao

# RHEL/Fedora
sudo dnf install -y dnf-plugins-core
sudo dnf config-manager --add-repo https://openbao.org/repo/rpm/openbao.repo
sudo dnf install openbao
```

Package install drops:
- Binary at `/usr/bin/bao`
- Config dir `/etc/openbao.d/`
- Systemd unit at `/usr/lib/systemd/system/openbao.service`
- Data dir `/opt/openbao/data`

### Binary (manual)

```bash
BAO_VERSION=2.2.0
curl -LO https://github.com/openbao/openbao/releases/download/v${BAO_VERSION}/bao_${BAO_VERSION}_linux_amd64.zip
curl -LO https://github.com/openbao/openbao/releases/download/v${BAO_VERSION}/bao_${BAO_VERSION}_SHA256SUMS
sha256sum --check --ignore-missing bao_${BAO_VERSION}_SHA256SUMS
unzip bao_${BAO_VERSION}_linux_amd64.zip && sudo mv bao /usr/local/bin/
```

### Docker

```bash
docker run --cap-add=IPC_LOCK \
  -e BAO_LOCAL_CONFIG='{"storage": {"file": {"path": "/bao/data"}}, "listener": [{"tcp": {"address": "0.0.0.0:8200", "tls_disable": 1}}], "ui": true}' \
  -p 8200:8200 \
  ghcr.io/openbao/openbao:latest server
```

Docker suitable for dev/test. **Not recommended for production** — key management and mlock are awkward in containers.

---

## 2. Production Configuration

### /etc/openbao.d/config.hcl (single-node production)

```hcl
ui = true

listener "tcp" {
  address       = "0.0.0.0:8200"
  tls_cert_file = "/etc/openbao/tls/fullchain.pem"
  tls_key_file  = "/etc/openbao/tls/privkey.pem"

  # Restrict to TLS 1.2+
  tls_min_version = "tls12"
}

storage "raft" {
  path    = "/opt/openbao/data"
  node_id = "node1"
  # performance_multiplier = 1  # set to 1 for production (default=5)
}

api_addr     = "https://bao.yourdomain.com:8200"
cluster_addr = "https://bao.yourdomain.com:8201"

# Prevent core dumps + memory swap
disable_mlock = false  # keep false; mlock enabled by default
```

### TLS

Use Let's Encrypt (certbot) or internal CA. Certbot renewal hook:

```bash
# /etc/letsencrypt/renewal-hooks/deploy/openbao.sh
cp /etc/letsencrypt/live/bao.yourdomain.com/fullchain.pem /etc/openbao/tls/
cp /etc/letsencrypt/live/bao.yourdomain.com/privkey.pem   /etc/openbao/tls/
chown openbao:openbao /etc/openbao/tls/*
systemctl reload openbao   # or: bao operator step-down then restart
```

---

## 3. Storage Backend: Raft vs Others

| Backend | HA | Complexity | Production Grade | Notes |
|---------|-----|------------|------------------|-------|
| **Raft (integrated)** | Yes | Low | Yes (recommended) | No external dependency |
| PostgreSQL | Yes (via HA PG) | Medium | Yes | Needs existing PG infra |
| Filesystem | No | Minimal | Single-node only | Dev/test |
| Consul | Yes | High | Yes (legacy) | Extra ops burden |

**Use Raft.** It's the current recommended approach — no external dependencies, built-in replication, snapshots included.

Key Raft tuning for production:

```hcl
storage "raft" {
  path                   = "/opt/openbao/data"
  node_id                = "node1"
  performance_multiplier = 1        # production: set to 1
  snapshot_threshold     = 8192
  trailing_logs          = 10000
}
```

---

## 4. Auto-Unseal Options

Without auto-unseal, every restart requires manual input of 3 of 5 Shamir keys — a significant operational burden.

### Option A: Transit Seal (self-hosted, recommended for VPS)

Use a **separate** OpenBao/Vault instance as the unsealer. Common pattern: one small "unsealer" instance with file storage, the main cluster configured to transit-unseal against it.

```hcl
seal "transit" {
  address         = "https://unsealer.internal:8200"
  token           = "s.xxxxxxxxxxxxxxxxx"  # periodic orphan token
  key_name        = "autounseal-key"
  mount_path      = "transit/"
  tls_ca_cert     = "/etc/openbao/tls/ca.pem"
}
```

Token must have policy:
```hcl
path "transit/encrypt/autounseal-key"  { capabilities = ["update"] }
path "transit/decrypt/autounseal-key"  { capabilities = ["update"] }
```

Use an **orphan, periodic token** (`bao token create -orphan -period=24h -policy=autounseal`) to avoid parent token expiry killing the seal.

### Option B: Cloud KMS

If running on cloud VPS with access to cloud KMS:

```hcl
# AWS KMS
seal "awskms" {
  region     = "us-east-1"
  kms_key_id = "arn:aws:kms:us-east-1:123456789012:key/..."
}

# GCP Cloud KMS
seal "gcpckms" {
  project    = "my-project"
  region     = "global"
  key_ring   = "my-key-ring"
  crypto_key = "openbao-unseal"
}
```

### Option C: PKCS#11 (HSM)

For high-security environments with HSM hardware. Config depends on HSM vendor.

### Shamir (manual) — when no auto-unseal

```bash
# After every restart:
bao operator unseal   # repeat 3x with different keys
```

Store keys in: password manager with multiple holders, printed + physically secured, or split between team members.

---

## 5. Systemd Service Setup

If installed via package manager, unit file already exists. For manual binary install:

```ini
# /etc/systemd/system/openbao.service
[Unit]
Description=OpenBao
Documentation=https://openbao.org/docs/
Requires=network-online.target
After=network-online.target
ConditionFileNotEmpty=/etc/openbao.d/config.hcl

[Service]
User=openbao
Group=openbao
ProtectSystem=full
ProtectHome=read-only
PrivateTmp=yes
PrivateDevices=yes
SecureBits=keep-caps
AmbientCapabilities=CAP_IPC_LOCK
CapabilityBoundingSet=CAP_SYSLOG CAP_IPC_LOCK
NoNewPrivileges=yes
ExecStart=/usr/bin/bao server -config=/etc/openbao.d/
ExecReload=/bin/kill --signal HUP $MAINPID
KillMode=process
KillSignal=SIGINT
Restart=on-failure
RestartSec=5
TimeoutStopSec=30
LimitNOFILE=65536
LimitMEMLOCK=infinity
MemorySwapMax=0

[Install]
WantedBy=multi-user.target
```

```bash
# Create user, dirs, permissions
sudo useradd --system --home /etc/openbao.d --shell /bin/false openbao
sudo mkdir -p /opt/openbao/data /etc/openbao/tls
sudo chown -R openbao:openbao /opt/openbao /etc/openbao.d

sudo systemctl daemon-reload
sudo systemctl enable openbao
sudo systemctl start openbao
```

**First-time init** (run once):
```bash
export BAO_ADDR='https://bao.yourdomain.com:8200'
bao operator init -key-shares=5 -key-threshold=3

# SAVE OUTPUT SECURELY — these are your unseal keys + root token
# If using auto-unseal: -key-shares=1 -key-threshold=1
```

---

## 6. Backup Strategy

### Raft Snapshot (primary method)

```bash
# Manual snapshot
BAO_ADDR=https://bao.yourdomain.com:8200
BAO_TOKEN=<root-or-operator-token>
bao operator raft snapshot save /backups/openbao-$(date +%Y%m%d-%H%M%S).snap
```

**Automated with cron:**

```bash
# /etc/cron.d/openbao-backup
0 2 * * * openbao /usr/local/bin/openbao-backup.sh >> /var/log/openbao-backup.log 2>&1
```

```bash
#!/bin/bash
# /usr/local/bin/openbao-backup.sh
set -euo pipefail
BACKUP_DIR=/backups/openbao
SNAPSHOT_FILE="${BACKUP_DIR}/snapshot-$(date +%Y%m%d-%H%M%S).snap"
mkdir -p "$BACKUP_DIR"

BAO_TOKEN=$(cat /etc/openbao/backup-token) \
BAO_ADDR=https://bao.yourdomain.com:8200 \
  bao operator raft snapshot save "$SNAPSHOT_FILE"

# Retain last 14 snapshots
ls -t "${BACKUP_DIR}"/*.snap | tail -n +15 | xargs -r rm --

# Sync to offsite (S3, rsync, etc.)
aws s3 cp "$SNAPSHOT_FILE" s3://your-bucket/openbao-backups/
```

**openbao-snapshot-agent** (official tool for automation):
https://github.com/openbao/openbao-snapshot-agent

Note: `snapshot` is NOT supported when Raft is used only as `ha_storage`.

### Restore

```bash
# Target server must be running and unsealed
BAO_TOKEN=<root-token> bao operator raft snapshot restore /backups/openbao-20260225.snap
```

Set `BAO_CLIENT_TIMEOUT` (e.g., `300s`) for large snapshots — default timeout may be too short.

---

## 7. HA Considerations

### Cluster Sizing

- **Single node**: Fine for most use cases. Raft requires no external deps.
- **3-node cluster**: Tolerates 1 node failure. Minimum for true HA.
- **5-node cluster**: Official recommendation. Tolerates 2 failures.

### 3-Node Raft Cluster Config (each node)

```hcl
storage "raft" {
  path    = "/opt/openbao/data"
  node_id = "node1"  # change per node: node1, node2, node3

  retry_join {
    leader_api_addr = "https://node1.internal:8200"
  }
  retry_join {
    leader_api_addr = "https://node2.internal:8200"
  }
  retry_join {
    leader_api_addr = "https://node3.internal:8200"
  }
}

api_addr     = "https://node1.internal:8200"   # change per node
cluster_addr = "https://node1.internal:8201"   # change per node
```

Init only on one node; others join via `retry_join`.

### Load Balancer

- Put all nodes behind a load balancer (HAProxy, nginx, or cloud LB)
- Active node handles writes; standbys redirect to active
- Health check: `GET /v1/sys/health` — returns 200 (active), 429 (standby), 503 (sealed)

HAProxy health check example:
```
option httpchk GET /v1/sys/health
http-check expect status 200
```

Or allow standbys to serve reads:
```
http-check expect rstatus 200|429
```

### Standby Behavior

- Unsealed standbys redirect client requests to the active node
- Sealed standbys drop out of rotation — unseal manually or via auto-unseal
- Auto-unseal is essentially required for any HA setup (avoids manual unsealing after node restart)

### Network Ports

| Port | Purpose |
|------|---------|
| 8200 | Client API (HTTPS) |
| 8201 | Cluster replication (TLS, internal only) |

Firewall: 8200 open to clients, 8201 open between cluster nodes only.

---

## Security Hardening Checklist

- [ ] TLS everywhere (client + cluster comms)
- [ ] `disable_mlock = false` (default) + `MemorySwapMax=0` in systemd
- [ ] Firewall: 8200 restricted, 8201 cluster-internal only
- [ ] Auto-unseal configured (transit or KMS)
- [ ] Root token revoked after initial setup; use AppRole/OIDC for apps
- [ ] Audit log enabled: `bao audit enable file file_path=/var/log/openbao/audit.log`
- [ ] Snapshots automated + offsite
- [ ] UI disabled if not needed: `ui = false`
- [ ] Rotate unseal keys periodically: `bao operator rekey`

---

## Sources

- [OpenBao Installation Docs](https://openbao.org/docs/install/)
- [OpenBao Configuration Reference](https://openbao.org/docs/configuration/)
- [Integrated Storage (Raft) Backend](https://openbao.org/docs/configuration/storage/raft/)
- [Seal Configuration](https://openbao.org/docs/configuration/seal/)
- [Transit Auto-Unseal](https://openbao.org/docs/configuration/seal/transit/)
- [High Availability Internals](https://openbao.org/docs/internals/high-availability/)
- [Raft Snapshot Commands](https://openbao.org/docs/commands/operator/raft/)
- [openbao-snapshot-agent](https://github.com/openbao/openbao-snapshot-agent)
- [Linode Deployment Guide](https://www.linode.com/docs/guides/deploying-openbao-on-a-linode-instance/)

---

## Unresolved Questions

1. **Auto-snapshot built-in**: Issue #795 tracks Raft auto-snapshotting natively; not yet merged as of research date — use cron + snapshot-agent for now.
2. **Transit unsealer HA**: If the transit unsealer goes down, the main cluster can't restart. Needs its own HA setup or use cloud KMS instead.
3. **openbao-snapshot-agent maturity**: Project exists but docs are thin — verify version compatibility with your OpenBao version before relying on it.
4. **PKCS11 / HSM support**: Available but vendor-specific; not covered here.
