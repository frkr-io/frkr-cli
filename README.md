# frkr-cli

Command-line tool for streaming and forwarding mirrored API traffic from frkr.

## Overview

The frkr CLI connects to the Streaming Gateway and forwards mirrored HTTP requests to local services, enabling live testing and debugging of production APIs.

## Installation

### From Source

```bash
git clone https://github.com/frkr-io/frkr-cli
cd frkr-cli
make build
```

The binary will be created in the `bin/` directory as `bin/frkr`.

To clean build artifacts:

```bash
make clean
```

### Using Go Install

```bash
go install github.com/frkr-io/frkr-cli/cmd/frkr@latest
```

## Authentication

Before streaming, you can authenticate using the OIDC login command. This supports standard OIDC providers like Auth0, Keycloak, etc.

### Login

```bash
./bin/frkr login --client-id <your-client-id>
```

This will open your default browser to perform the authentication flow. Upon success, a token will be stored in `~/.frkr/auth.json`.

### Configuration

You can configure the login command via flags or a YAML configuration file.

**Flags:**
*   `--client-id`: (Required) OIDC Client ID.
*   `--auth-domain`: OIDC Issuer URL (Default: `https://auth.frkr.io`).
*   `--audience`: JWT Audience (Default: `https://api.frkr.io`).
*   `--callback-port`: Local port for the auth callback (Default: `38911`).

**Configuration File (`frkr-login.yaml`):**

To avoid passing flags every time, you can place a `frkr-login.yaml` file in your working directory:

```yaml
client_id: "your-client-id"
auth_domain: "https://your-domain.auth0.com"
audience: "https://api.your-app.com"
callback_port: "38911"
```

Then simply run:

```bash
./bin/frkr login
```

You can also specify a custom config file path:

```bash
./bin/frkr login --config /path/to/my-config.yaml
```

> **Note**: Flags take precedence over properites in the configuration file if both are provided (and you are NOT using the `--config` flag). If `--config` is used, it is mutually exclusive with individual auth flags.

## Usage

### Stream and Forward Requests

Once authenticated (or if using manual credentials), you can stream requests.

```bash
# Using stored login credentials
./bin/frkr stream <stream-id> --forward-url=http://localhost:3001

# Using manual credentials (overrides stored login)
./bin/frkr stream <stream-id> \
  --username=testuser \
  --password=testpass \
  --forward-url=http://localhost:3001
```

### Command Options

| Option | Default | Description |
|--------|---------|-------------|
| `--gateway-url` | `http://localhost:8081` | Streaming Gateway URL |
| `--username` | (none) | Username for basic authentication |
| `--password` | (none) | Password for basic authentication |
| `--forward-url` | `http://localhost:3001` | URL to forward requests to |
| `--forward-timeout` | `30` | Timeout in seconds for forwarding requests |
| `--max-retries` | `3` | Maximum number of retries for failed forwards |

## Examples

### Authenticated Stream

1.  **Login**: `frkr login` (uses `frkr-login.yaml`)
2.  **Stream**: `frkr stream my-api`

### Manual Auth Stream

Stream messages from `my-api` and forward them to a local service using specific credentials:

```bash
./bin/frkr stream my-api \
  --gateway-url=http://localhost:8081 \
  --username=testuser \
  --password=testpass \
  --forward-url=http://localhost:3001
```

### Custom Timeout and Retries

```bash
./bin/frkr stream my-api \
  --forward-timeout=60 \
  --max-retries=5
```

## How It Works

1. **Authentication**: 
   - `frkr login` performs a PKCE authorization code flow.
   - Saves access/refresh tokens to `~/.frkr/auth.json`.
2. **Streaming**: 
   - Connects to the Streaming Gateway using Server-Sent Events (SSE).
   - Uses the stored Bearer token (or provided Basic Auth).
3. **Forwarding**:
   - Receives mirrored HTTP requests as JSON messages.
   - Reconstructs HTTP requests and forwards them to the specified URL.
   - Retries failed forwards with exponential backoff.

## Error Handling

The CLI includes automatic retry logic with exponential backoff:

- **Transient errors** (network timeouts, connection failures) are retried up to `--max-retries` times
- **Client errors** (4xx status codes) are not retried
- **Server errors** (5xx status codes) are logged but considered handled

## Requirements

- Go 1.21 or later (for building from source)
- Access to a running Streaming Gateway
- Local service to forward requests to

## License

Apache 2.0
