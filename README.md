# Notes App

A production-grade Go web application for creating and managing markdown notes.

The frontend lives in a separate repo:
[notes-webpage](https://github.com/bscheibe/notes-webpage), a static site on
Firebase Hosting that calls this service as a JSON API.

## Project Structure

This project follows idiomatic Go project layout and architecture patterns:

```
notes-app/
├── cmd/
│   └── server/
│       └── main.go        # Application entry point
├── docs/                  # Documentation (see docs/README.md for index)
├── infra/                 # Terraform-managed Cloud Run infrastructure
├── internal/
│   ├── auth/               # OAuth clients, auth service, user repository
│   ├── config/              # Configuration management
│   ├── handlers/            # HTTP handlers
│   ├── middleware/          # Auth middleware
│   ├── models/              # Data models
│   ├── monitoring/          # Metrics, tracing, health checks
│   ├── repository/          # Data access layer
│   ├── server/              # Server setup and routing
│   └── service/             # Business logic layer
├── config.prod.yaml       # Production configuration
├── config.staging.yaml    # Staging configuration
├── Dockerfile
├── go.mod                 # Go module file
└── README.md              # This file
```

## Architecture

The application follows a clean architecture pattern with clear separation of concerns:

- **Config Layer**: Handles configuration with Go defaults, optional YAML files, and environment variable overrides
- **Repository Layer**: Manages file system operations (data access)
- **Service Layer**: Contains business logic and validation
- **Handler Layer**: Handles HTTP requests and responses
- **Server Layer**: Sets up routing, middleware, and server configuration

## Documentation

Detailed documentation is available in [`docs/`](docs/README.md), covering authentication architecture, integration testing, and Cloud Run deployment.

## Getting Started

### Prerequisites

- Go 1.26 or higher
- Make (optional, for build scripts)

### Installation

1. Clone the repository
2. Install dependencies:
   ```bash
   go mod download
   ```

### Configuration

The application uses a three-tier configuration approach:

1. **Go defaults** (in `internal/config/config.go`): Sensible defaults for local development
2. **Config files** (optional): Environment-specific settings (staging, production)
3. **Environment variables** (highest priority): Runtime overrides and secrets

#### Default Configuration

The application works out-of-the-box with built-in defaults:
- Server: `localhost:8080`
- Notes directory: System temp directory
- Logging: JSON format, info level
- Monitoring: Enabled with tracing

#### Environment-Specific Configuration

For different environments, create config files:

**config.staging.yaml**:
```yaml
notes:
  directory: "/mnt/staging/notes-app"
```

**config.prod.yaml**:
```yaml
notes:
  directory: "/var/lib/notes-app"
```

#### Environment Variables

Override any setting with environment variables (prefix: `NOTES_APP_`):

```bash
export NOTES_APP_SERVER_PORT=9000
export NOTES_APP_LOGGING_LEVEL=debug
export NOTES_APP_NOTES_DIRECTORY=/custom/path
```

Note: Use nested keys with underscores: `NOTES_APP_SERVER_PORT` maps to `server.port`

### Running the Application

```bash
# Run with defaults (local development)
go run cmd/server/main.go

# Run with environment-specific config
go run cmd/server/main.go --config config.prod.yaml

# Build and run
go build -o notes-app cmd/server/main.go
./notes-app

# Run with environment variable overrides
NOTES_APP_SERVER_PORT=9000 ./notes-app
```

The server will start on `http://localhost:8080` (or configured port)

## Development

### Project Layout Rationale

- **cmd/server/**: Entry point for the server application
- **internal/**: Private application code (not importable by other projects)
- **models/**: Data structures and DTOs
- **repository/**: Data access abstraction (makes testing easier)
- **service/**: Business logic (can be tested independently of HTTP)
- **handlers/**: HTTP request/response handling
- **config/**: Configuration management

### Key Design Decisions

1. **Clean Architecture**: Separation of concerns with distinct layers
2. **Dependency Injection**: Manual constructor-based DI (idiomatic Go)
3. **Configuration Management**: Viper for flexible config from files/env
4. **Structured Logging**: slog for contextual, searchable logs
5. **Graceful Shutdown**: Proper handling of SIGINT/SIGTERM
6. **Chi Router**: Lightweight, idiomatic HTTP router

### Adding New Features

1. **Add model** in `internal/models/`
2. **Add repository methods** in `internal/repository/`
3. **Add service methods** in `internal/service/`
4. **Add handler methods** in `internal/handlers/`
5. **Register routes** in `internal/server/server.go`

### Testing

Run Go unit and integration tests:
```bash
go test ./...
```

Run tests with coverage:
```bash
go test -cover ./...
```

For detailed information about the testing approach, see the
[Integration Testing documentation](docs/INTEGRATION_TESTING.md).

## Production Considerations

This application includes several production-ready features:

- ✅ Structured logging with slog
- ✅ Configuration management with Viper
- ✅ Graceful shutdown handling
- ✅ Request timeout middleware
- ✅ Panic recovery middleware
- ✅ Request ID tracking
- ✅ Real IP detection
- ✅ Error handling at each layer
- ✅ Clean architecture for testability
- ✅ **OpenTelemetry observability**
- ✅ **Prometheus metrics**
- ✅ **Distributed tracing**
- ✅ **Health check endpoints**

## Monitoring & Observability

The application includes industry-standard monitoring with OpenTelemetry:

### Metrics (Prometheus)
- **Endpoint**: `GET /metrics`
- **Application metrics**:
  - `notes_created_total` - Total notes created
  - `notes_updated_total` - Total notes updated
  - `notes_deleted_total` - Total notes deleted
  - `notes_read_errors_total` - Note read errors
  - `notes_write_errors_total` - Note write errors
- **System metrics**: Memory, goroutines, GC, etc.

### Health Checks
- **`GET /health`** - Detailed health status with all checks
- **`GET /healthz`** - Readiness probe (Kubernetes style)
- **`GET /livez`** - Liveness probe (Kubernetes style)

### Distributed Tracing
- OpenTelemetry tracing for request flow
- stdout exporter for development (replace with Jaeger/OTLP in production)
- Automatic span creation for HTTP handlers

### Configuration
```yaml
monitoring:
  enabled: true
  service_name: "notes-app"
  tracing_enabled: true
```

### Production Monitoring Stack
For production deployment, integrate with:
- **Prometheus** - Metrics collection and alerting
- **Grafana** - Visualization and dashboards
- **Jaeger** - Distributed tracing (replace stdout exporter)
- **Loki** - Log aggregation

## Future Enhancements

Potential improvements for production deployment:

- Add database integration (PostgreSQL, etc.)
- Add API versioning
- Add rate limiting

## License

[MIT](LICENSE)