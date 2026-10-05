package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// healthcheckTimeout bounds one probe, below Docker's HEALTHCHECK timeout.
const healthcheckTimeout = 3 * time.Second

// probeURL returns the /readyz URL for a server listening on addr. An empty
// or unspecified host (":8080", "0.0.0.0:8080", "[::]:8080") is probed on
// loopback of the same family.
//
// Parameters:
//   - addr: the GONE_ADDR listen address, already validated.
//
// Returns:
//   - string: the readiness URL.
//   - error: non-nil if addr is not host:port.
func probeURL(addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("listen address %q: %w", addr, err)
	}
	ip := net.ParseIP(host)
	switch {
	case host == "" || (ip != nil && ip.Equal(net.IPv4zero)):
		host = "127.0.0.1"
	case ip != nil && ip.Equal(net.IPv6unspecified):
		host = "::1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/readyz", nil
}

// probe asks the server at url whether it is ready.
//
// Parameters:
//   - ctx: bounds the request.
//   - client: the HTTP client to use.
//   - url: the readiness URL.
//
// Returns:
//   - error: nil only for a 200 response.
func probe(ctx context.Context, client *http.Client, url string) (err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, resp.Body.Close()) }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned %s", url, resp.Status)
	}
	return nil
}

// healthcheck implements "goned healthcheck": it probes this server's
// /readyz so a container HEALTHCHECK works without a shell or curl in the
// image. It reads the same GONE_ environment as the server.
//
// Parameters:
//   - stderr: where a failure is reported.
//
// Returns:
//   - int: the process exit code, 0 when ready and 1 otherwise.
func healthcheck(stderr io.Writer) int {
	err := runHealthcheck()
	if err != nil {
		fmt.Fprintln(stderr, "goned healthcheck:", err)
		return 1
	}
	return 0
}

// runHealthcheck loads the listen address and probes it.
//
// Returns:
//   - error: non-nil if the configuration is invalid or the server is not
//     ready.
func runHealthcheck() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	url, err := probeURL(cfg.Addr)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), healthcheckTimeout)
	defer cancel()
	return probe(ctx, &http.Client{}, url)
}
