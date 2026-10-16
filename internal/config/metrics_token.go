package config

// minMetricsTokenLen is the recommended shortest GONE_METRICS_TOKEN. The token
// is the only credential on the metrics listener, so a guessable one such as
// "change-me" is weak. A shorter token still works for now; a future release
// will refuse it.
const minMetricsTokenLen = 16

// metricsTokenWarning is logged once at startup for a short metrics token. It
// never includes the token.
const metricsTokenWarning = "GONE_METRICS_TOKEN is shorter than 16 characters; a future release will refuse it; generate one with: openssl rand -base64 24"

// warnings returns the non-fatal findings for cfg. An empty token (metrics
// off) is not a finding.
//
// Parameters:
//   - cfg: the validated configuration.
//
// Returns:
//   - []string: warning messages, nil when there are none.
func warnings(cfg *Config) []string {
	if cfg.MetricsToken != "" && len(cfg.MetricsToken) < minMetricsTokenLen {
		return []string{metricsTokenWarning}
	}
	return nil
}
