package owner

import (
	"context"
	"log/slog"
)

// EnvOTLPEndpoint turns OpenTelemetry on (SDD-000 section 10, ADR-0026 item 4).
const EnvOTLPEndpoint = "OTEL_EXPORTER_OTLP_ENDPOINT"

// Telemetry builds the OpenTelemetry exporter and returns the function that
// flushes and stops it. It is the seam for vukem's drivers/otel: the owner calls
// it only when OTEL_EXPORTER_OTLP_ENDPOINT is set, so a default start constructs
// no exporter and opens no connection (SDD-007 section 10.1 path 3, T-10).
type Telemetry func(ctx context.Context, getenv func(string) string) (shutdown func(context.Context) error, err error)

// startTelemetry runs the Telemetry hook when the operator asked for it. Without
// a hook the request is logged and ignored: nothing is sent anywhere.
func (o *Owner) startTelemetry(ctx context.Context, cfg Config) error {
	if cfg.Getenv(EnvOTLPEndpoint) == "" {
		return nil
	}

	if cfg.Telemetry == nil {
		cfg.Log.WarnContext(ctx, "OpenTelemetry is not started: this build has no exporter",
			slog.String("variable", EnvOTLPEndpoint))

		return nil
	}

	shutdown, err := cfg.Telemetry(ctx, cfg.Getenv)
	if err != nil {
		return err
	}

	o.telemetry = shutdown

	return nil
}
