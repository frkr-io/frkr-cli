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

## Usage

### Stream and Forward Requests

```bash
./bin/frkr stream <stream-id> \
  --gateway-url=http://localhost:8081 \
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

### Basic Usage

Stream messages from `my-api` and forward them to a local service:

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
  --gateway-url=http://localhost:8081 \
  --username=testuser \
  --password=testpass \
  --forward-url=http://localhost:3001 \
  --forward-timeout=60 \
  --max-retries=5
```

## How It Works

1. Connects to the Streaming Gateway using Server-Sent Events (SSE)
2. Authenticates using Basic Authentication
3. Receives mirrored HTTP requests as JSON messages
4. Reconstructs HTTP requests and forwards them to the specified URL
5. Retries failed forwards with exponential backoff

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
