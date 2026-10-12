package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/config"
)

// healthcheckTimeout bounds the whole check: the image's HEALTHCHECK
// waits 5 seconds for the command.
const healthcheckTimeout = 3 * time.Second

// healthcheck asks /healthz of the server at the address POLYFIN_LISTEN
// gives, and returns nil when it answers 200 within timeout. It is the
// image's HEALTHCHECK, which has no HTTP client of its own.
func healthcheck(getenv func(string) string, timeout time.Duration) error {
	address, err := healthcheckAddress(getenv("POLYFIN_LISTEN"))
	if err != nil {
		return err
	}
	client := &http.Client{
		Timeout: timeout,
		// The server is local: no proxy of the environment stands between.
		Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true},
	}
	target := (&url.URL{Scheme: "http", Host: address, Path: "/healthz"}).String()
	response, err := client.Get(target)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %s", target, response.Status)
	}
	return nil
}

// healthcheckAddress is the address to reach a server listening at
// listen: the loopback one when listen names no host, or every address.
func healthcheckAddress(listen string) (string, error) {
	listen = strings.TrimSpace(listen)
	if listen == "" {
		listen = config.DefaultListen
	}
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "", fmt.Errorf("POLYFIN_LISTEN %q is not a host:port address", listen)
	}
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port), nil
}
