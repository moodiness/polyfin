package netproxy

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()
	for _, valid := range []string{
		"socks5://warp:1080",
		"socks5h://warp:1080",
		"http://127.0.0.1:8080",
		"https://proxy.example.org:8443",
		"  socks5h://warp:1080  ",
	} {
		if u, err := Parse(valid); err != nil || u == nil {
			t.Errorf("Parse(%q) = %v, %v", valid, u, err)
		}
	}
	for _, invalid := range []string{
		"", "warp:1080", "ftp://warp:1080", "socks5://warp", "socks5://warp:0",
		"socks5://warp:65536", "socks5://warp:1080/path", "socks5://warp:1080?x=1",
		"socks5://warp:1080#fragment", "socks5://:1080", "socks5:// warp:1080",
	} {
		if u, err := Parse(invalid); !errors.Is(err, ErrInvalidProxy) || u != nil {
			t.Errorf("Parse(%q) = %v, %v, want ErrInvalidProxy", invalid, u, err)
		}
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()
	if !Validate("") {
		t.Error("an empty proxy is valid")
	}
	for raw, want := range map[string]bool{
		"socks5h://warp:1080": true,
		"not a proxy":         false,
	} {
		if got := Validate(raw); got != want {
			t.Errorf("Validate(%q) = %t, want %t", raw, got, want)
		}
	}
}

func TestDetect(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	got, ok := Detector{Candidates: []string{"127.0.0.1"}, Port: port}.Detect(t.Context())
	if !ok || got != "socks5h://127.0.0.1:"+port {
		t.Fatalf("Detect = %q, %t, want socks5h://127.0.0.1:%s, true", got, ok, port)
	}
}

func TestDetectNothing(t *testing.T) {
	t.Parallel()
	got, ok := Detector{
		Candidates: []string{"warp", "warp-svc"},
		Dial: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("no answer")
		},
	}.Detect(t.Context())
	if ok || got != "" {
		t.Fatalf("Detect = %q, %t, want no detection", got, ok)
	}
}

func TestDefaultCandidates(t *testing.T) {
	t.Parallel()
	if len(DefaultCandidates) == 0 {
		t.Fatal("no candidates")
	}
	for _, name := range DefaultCandidates {
		if strings.TrimSpace(name) == "" {
			t.Error("an empty candidate")
		}
	}
}
