---
title: "TLS, HTTPS and mutual TLS (mTLS)"
slug: "docs/tls"
description: "How to enable HTTPS and certificate-based client authentication on the MDDB HTTP listener — config, cert generation, deployment recipes, troubleshooting."
status: publish
---

# TLS, HTTPS and mutual TLS (mTLS)

MDDB ships with a built-in TLS/HTTPS listener for the HTTP API and an
optional mutual-TLS mode where every client must present a certificate signed
by a CA you control. The same code path serves both: turn on `MDDB_TLS_ENABLED`
and provide a server cert/key, then optionally point `MDDB_TLS_CLIENT_CA` at a
trusted-CA bundle to require client certificates.

This document covers config, generating certs with `openssl`, common
deployment recipes, and the most frequent gotchas.

> **Scope**: TLS is configured on **both** the HTTP listener
> ([services/mddbd/main.go](../services/mddbd/main.go) — `ServeTLS`) and the
> gRPC listener (`grpc.Creds(credentials.NewTLS(...))` in the same file).
> Both reuse the same `tls.Config` built by `buildServerTLSConfig` in
> [services/mddbd/tls_config.go](../services/mddbd/tls_config.go), so a single
> `tls.*` config block enables HTTPS and TLS-secured gRPC simultaneously, and
> the same `clientCAFile` / `clientAuth` settings enable mTLS on both. The
> Unix Domain Socket listener (see [config.md](config.md#unix-domain-socket-transport))
> always runs in plaintext on either protocol — filesystem permissions
> authenticate the local peer, and API keys / JWT still apply on top.

## Quick start (HTTPS only)

```bash
MDDB_TLS_ENABLED=true \
MDDB_TLS_CERT=/etc/mddb/server.crt \
MDDB_TLS_KEY=/etc/mddb/server.key \
./mddbd
```

The startup log line confirms TLS is on:

```
mddb HTTPS listening on :11023 (mode=wr, db=mddb.db, tls=on)
```

Test with `curl`:

```bash
curl --cacert /etc/mddb/ca.crt https://mddb.example.com/v1/healthz
```

## Quick start (mTLS — clients must present certificates)

```bash
MDDB_TLS_ENABLED=true \
MDDB_TLS_CERT=/etc/mddb/server.crt \
MDDB_TLS_KEY=/etc/mddb/server.key \
MDDB_TLS_CLIENT_CA=/etc/mddb/clients-ca.crt \
MDDB_TLS_CLIENT_AUTH=require \
./mddbd
```

Startup log line:

```
mddb HTTPS listening on :11023 (mode=wr, db=mddb.db, tls=on, mtls=on (clientAuth=require))
```

Now any unauthenticated TLS handshake is rejected:

```bash
$ curl --cacert /etc/mddb/ca.crt https://mddb.example.com/v1/healthz
curl: (56) OpenSSL SSL_read: error:0A00045C:SSL routines::tlsv13 alert certificate required, errno 0
```

Client succeeds when presenting a cert chained to `clients-ca.crt`:

```bash
curl --cacert /etc/mddb/ca.crt \
     --cert /etc/mddb/clients/alice.crt \
     --key  /etc/mddb/clients/alice.key \
     https://mddb.example.com/v1/healthz
```

## Configuration reference

| Env var | Default | Type | Description |
|---|---|---|---|
| `MDDB_TLS_ENABLED` | `false` | bool | Master switch for HTTPS on the HTTP listener |
| `MDDB_TLS_CERT` | `""` | string | Path to server certificate (PEM) |
| `MDDB_TLS_KEY` | `""` | string | Path to server private key (PEM) |
| `MDDB_TLS_CLIENT_CA` | `""` | string | Path to PEM bundle of trusted client CAs. Setting this enables **mTLS** |
| `MDDB_TLS_CLIENT_AUTH` | `"require"` | string | mTLS mode: `require` (reject anonymous) or `request` (verify if presented) |

YAML equivalents (`config.yaml`):

```yaml
tls:
  enabled: true
  certFile: /etc/mddb/server.crt
  keyFile: /etc/mddb/server.key
  clientCAFile: /etc/mddb/clients-ca.crt
  clientAuth: require   # or "request"
```

Implementation lives in [services/mddbd/tls_config.go](../services/mddbd/tls_config.go)
(`buildServerTLSConfig`). Key facts:

- `MinVersion` is pinned to **TLS 1.2** — older clients are rejected at handshake.
- `clientAuth: require` maps to `tls.RequireAndVerifyClientCert`.
- `clientAuth: request` maps to `tls.VerifyClientCertIfGiven` — useful for
  staged rollouts where some clients have certs and others fall back to JWT.
- The server cert + key are loaded once at startup and held in
  `http.Server.TLSConfig`. To rotate, restart the process (or use a SIGHUP
  reloader you wire externally).

## Generating certificates with `openssl`

The minimal demo CA + server cert + client cert below is enough to bring up a
mTLS deployment locally. For production use a real CA (Let's Encrypt for the
server cert; an internal CA or step-ca for client certs).

### 1. CA root (one-time)

```bash
mkdir -p /etc/mddb && cd /etc/mddb

# Root CA private key
openssl genrsa -out ca.key 4096

# Self-signed root certificate (valid 10 years)
openssl req -new -x509 -key ca.key -out ca.crt -days 3650 \
  -subj "/CN=MDDB Internal CA/O=Acme Inc/C=US"
```

### 2. Server certificate

```bash
# Server private key
openssl genrsa -out server.key 4096

# CSR with SAN (hostname + IP) — required by modern clients
openssl req -new -key server.key -out server.csr \
  -subj "/CN=mddb.example.com/O=Acme Inc/C=US" \
  -addext "subjectAltName=DNS:mddb.example.com,DNS:localhost,IP:127.0.0.1"

# Sign with the CA, copy SANs into the final cert
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out server.crt -days 825 \
  -extfile <(printf "subjectAltName=DNS:mddb.example.com,DNS:localhost,IP:127.0.0.1")

chmod 600 server.key
```

### 3. Client certificate (mTLS only)

```bash
mkdir -p /etc/mddb/clients && cd /etc/mddb/clients

openssl genrsa -out alice.key 4096
openssl req -new -key alice.key -out alice.csr \
  -subj "/CN=alice@example.com/O=Acme Inc/C=US"

openssl x509 -req -in alice.csr -CA ../ca.crt -CAkey ../ca.key -CAcreateserial \
  -out alice.crt -days 365

chmod 600 alice.key
```

Repeat step 3 for every client identity. The MDDB server only needs
`/etc/mddb/ca.crt` (or a bundle of multiple CAs) — it never sees the
individual client keys. Revoke a client by removing its CA from the bundle or
by maintaining a separate CRL outside MDDB (CRL/OCSP checking is not built
into MDDB at the moment).

### 4. Bundle of trusted client CAs

If you trust multiple CAs (e.g. one per business unit) just concatenate them:

```bash
cat ca-acme.crt ca-bizdev.crt ca-eng.crt > /etc/mddb/clients-ca.crt
```

`MDDB_TLS_CLIENT_CA` accepts a single PEM file containing one or more
certificates.

## Deployment recipes

### Behind a reverse proxy that terminates TLS

If nginx/Caddy/Cloudflare already terminates TLS, leave `MDDB_TLS_ENABLED=false`
and run MDDB on plain HTTP (or even better, on a Unix Domain Socket — see
[config.md UDS section](config.md#unix-domain-socket-transport)). The proxy
forwards to MDDB over loopback or the socket.

This is the simplest production setup and the one we recommend unless you
need direct mTLS into MDDB.

### Direct HTTPS, no proxy

Use `MDDB_TLS_ENABLED=true` with a cert from Let's Encrypt (renewed via
certbot or similar). Restart MDDB on cert renewal — there is no in-process
hot reload yet.

### mTLS for service-to-service

Run MDDB with `MDDB_TLS_CLIENT_CA=/etc/mddb/services-ca.crt`,
`MDDB_TLS_CLIENT_AUTH=require`. Issue one client cert per service via your
internal PKI. The server has no idea what `CN=alice@example.com` *is* — it
only verifies the chain. Couple this with API keys / JWT for app-level
identity if you need fine-grained per-service permissions.

### Staged rollout: mTLS optional

Set `MDDB_TLS_CLIENT_AUTH=request`. Clients that present a valid cert get
through; clients that don't fall through to your JWT or API-key middleware.
Once all clients have certs, flip to `require`.

## Troubleshooting

**`tls: failed to find any PEM data in certificate input`**
The cert file is empty, has wrong permissions, or contains DER instead of
PEM. Verify with `openssl x509 -in server.crt -noout -text`.

**`tls: certificate signed by unknown authority`** (client side)
The client doesn't trust the server's CA. Pass `--cacert /etc/mddb/ca.crt`
to curl, or install the CA cert in the OS trust store.

**`alert certificate required`** (client side, mTLS)
The server is in `clientAuth=require` mode and the client did not present a
certificate. Pass `--cert client.crt --key client.key` to curl.

**`tls: client didn't provide a certificate`** (server log)
Same as above from the server's perspective. Either the client config is
wrong, or you want to relax `MDDB_TLS_CLIENT_AUTH` to `request`.

**`x509: certificate has expired or is not yet valid`**
The cert is past its `notAfter` date or the system clock is wrong. Re-issue
with `openssl x509 -req ... -days 365`.

**`tls: TLS handshake error from <ip>: tls: no application protocol`**
Some client libraries require explicit ALPN. MDDB does not advertise
specific ALPN protocols on the HTTP/1.1 + HTTP/2 listener; if your client is
strict, pin it to `h2` or fall back to a plain HTTP request through the
proxy.

**Cert pinning**
MDDB does not support certificate pinning out of the box. If you need it,
terminate TLS at a proxy (Envoy, nginx) and let it enforce the pin.

## TLS on the gRPC listener

The gRPC server reuses the same `tls.Config` as the HTTP listener, so a single
`tls.*` config block enables HTTPS and TLS-secured gRPC at the same time, and
`MDDB_TLS_CLIENT_CA` enables mTLS on both. Startup log line:

```
mddb gRPC listening on :11024 (mode=wr, db=mddb.db, tls=on, mtls=on (clientAuth=require))
```

### Go client (`google.golang.org/grpc`)

```go
import (
    "crypto/tls"
    "crypto/x509"
    "os"

    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials"
)

caPEM, _ := os.ReadFile("/etc/mddb/ca.crt")
caPool := x509.NewCertPool()
caPool.AppendCertsFromPEM(caPEM)

// HTTPS-only (no client cert)
creds := credentials.NewTLS(&tls.Config{RootCAs: caPool})

// mTLS (client cert + key)
clientCert, _ := tls.LoadX509KeyPair("/etc/mddb/clients/alice.crt", "/etc/mddb/clients/alice.key")
creds = credentials.NewTLS(&tls.Config{
    RootCAs:      caPool,
    Certificates: []tls.Certificate{clientCert},
})

conn, _ := grpc.NewClient("mddb.example.com:11024", grpc.WithTransportCredentials(creds))
```

### Python client (`grpcio`)

```python
import grpc

with open("/etc/mddb/ca.crt", "rb") as f: ca = f.read()
with open("/etc/mddb/clients/alice.crt", "rb") as f: cert = f.read()
with open("/etc/mddb/clients/alice.key", "rb") as f: key = f.read()

creds = grpc.ssl_channel_credentials(
    root_certificates=ca,
    private_key=key,           # omit for HTTPS-only
    certificate_chain=cert,    # omit for HTTPS-only
)
channel = grpc.secure_channel("mddb.example.com:11024", creds)
```

### Node client (`@grpc/grpc-js`)

```js
const grpc = require('@grpc/grpc-js');
const fs = require('fs');

const creds = grpc.credentials.createSsl(
    fs.readFileSync('/etc/mddb/ca.crt'),
    fs.readFileSync('/etc/mddb/clients/alice.key'),     // null for HTTPS-only
    fs.readFileSync('/etc/mddb/clients/alice.crt'),     // null for HTTPS-only
);
const client = new MDDBClient('mddb.example.com:11024', creds);
```

### `grpcurl`

```bash
# HTTPS-only
grpcurl -cacert /etc/mddb/ca.crt mddb.example.com:11024 list

# mTLS
grpcurl -cacert /etc/mddb/ca.crt \
        -cert /etc/mddb/clients/alice.crt \
        -key  /etc/mddb/clients/alice.key \
        mddb.example.com:11024 mddb.MDDB/Stats
```

## What's not covered yet

These are deliberate omissions in the current implementation:

- **Hot reload of certs** without restart. Plan: SIGHUP-driven reloader. PR welcome.
- **Built-in CRL / OCSP checking**. Use a proxy or revoke at the CA bundle level.
- **TLS on UDS**. Intentional — UDS is local-only and authenticated by filesystem permissions.

See also:

- [config.md](config.md) — full env var reference, including the UDS section.
- [AUTHENTICATION.md](AUTHENTICATION.md) — JWT / API keys / RBAC. mTLS pairs well with JWT for layered auth.
- [DEPLOYMENT.md](DEPLOYMENT.md) — production deployment patterns.
