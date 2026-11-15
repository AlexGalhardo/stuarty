package owner

import (
	"log/slog"

	vukem "github.com/AlexGalhardo/vukem"
	"github.com/AlexGalhardo/vukem/drivers/otel"
)

// EnvOTLPEndpoint turns OpenTelemetry on (SDD-000 section 10, ADR-0026 item 4).
const EnvOTLPEndpoint = "OTEL_EXPORTER_OTLP_ENDPOINT"

// Telemetry builds the vukem provider that traces and measures the application.
// It is the seam for vukem's drivers/otel: the owner calls it only when
// OTEL_EXPORTER_OTLP_ENDPOINT is set, so a default start constructs no provider
// and opens no connection (SDD-007 section 10.1 path 3, T-10). The application
// registers the provider and shuts it down, which flushes what is pending.
type Telemetry func(version string) vukem.Provider

// OTelProvider is the Telemetry of the product: drivers/otel, which exports
// traces and metrics over OTLP/HTTP to the endpoint of the operator and nowhere
// else. The exporter itself connects on its first export, not here.
func OTelProvider(version string) vukem.Provider {
	return otel.New(otel.Config{ServiceVersion: version})
}

// TelemetryProvider is the provider to pass to vukem.WithProviders, or nil when
// the operator did not ask for OpenTelemetry.
func (o *Owner) TelemetryProvider() vukem.Provider { return o.telemetry }

// startTelemetry runs the Telemetry hook when the operator asked for it. Without
// a hook the request is logged and ignored: nothing is sent anywhere.
func (o *Owner) startTelemetry(cfg Config) {
	if cfg.Getenv(EnvOTLPEndpoint) == "" {
		return
	}

	if cfg.Telemetry == nil {
		cfg.Log.Warn("OpenTelemetry is not started: this build has no exporter",
			slog.String("variable", EnvOTLPEndpoint))

		return
	}

	o.telemetry = cfg.Telemetry(cfg.Version)
}
