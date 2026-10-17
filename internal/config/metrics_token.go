package config

// minMetricsTokenLen is the recommended shortest GONE_METRICS_TOKEN. The token
// is the only credential on the metrics listener, so a guessable one such as
// "change-me" is weak. A shorter token still works for now; a future release
// will refuse it.
const minMetricsTokenLen = 16

// metricsTokenWarning is logged once at startup for a short metrics token. It
// never includes the token.
const metricsTokenWarning = "GONE_METRICS_TOKEN is shorter than 16 characters; a future release will refuse it; generate one with: openssl rand -base64 24" // #nosec G101 -- a log message naming the variable, not a credential

// warnings returns the non-fatal findings for cfg: a short metrics token
// (an empty token means metrics are off, not a finding) and the sign-in
// warnings.
//
// Parameters:
//   - cfg: the validated configuration.
//
// Returns:
//   - []string: warning messages, nil when there are none.
func warnings(cfg *Config) []string {
	var out []string
	if cfg.MetricsToken != "" && len(cfg.MetricsToken) < minMetricsTokenLen {
		out = append(out, metricsTokenWarning)
	}
	return append(out, authWarnings(cfg)...)
}
