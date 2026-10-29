package client

import (
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"syscall"
	"testing"
	"time"
)

// timeoutErr is a net.Error that reports a timeout, standing in for the
// transport's own unexported timeout errors.
type timeoutErr struct{ msg string }

func (e timeoutErr) Error() string { return e.msg }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

var _ net.Error = timeoutErr{}

// TestIsRetryableTimeouts pins which transport timeouts are retried: a dial
// or TLS-handshake timeout never reached the cluster, so trying again is
// safe; a response timeout means the cluster is still working on the query,
// and retrying it would only run the expensive query again.
func TestIsRetryableTimeouts(t *testing.T) {
	get := func(err error) error {
		return &url.Error{Op: "Get", URL: "https://managed.example.com/e/env1/api/v2/problems", Err: err}
	}
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"dial timeout", get(&net.OpError{Op: "dial", Net: "tcp", Err: timeoutErr{"i/o timeout"}}), true},
		{"TLS handshake timeout", get(timeoutErr{"net/http: TLS handshake timeout"}), true},
		{"response timeout", get(timeoutErr{"context deadline exceeded (Client.Timeout exceeded while awaiting headers)"}), false},
		{"read timeout", get(&net.OpError{Op: "read", Net: "tcp", Err: timeoutErr{"i/o timeout"}}), false},
		{"connection reset", get(&net.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("read", syscall.ECONNRESET)}), true},
		{"truncated response", get(io.ErrUnexpectedEOF), true},
		{"connection refused", get(&net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}), false},
		{"other", errors.New("boom"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRetryable(nil, tt.err); got != tt.want {
				t.Errorf("isRetryable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestRequestTimeoutIs60s pins the per-attempt timeout of both clients. At
// 30s it cut off long queries that the cluster would have answered.
func TestRequestTimeoutIs60s(t *testing.T) {
	c, err := New("https://managed.example.com", "env1", "dt0c01.TEST")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.http.GetClient().Timeout; got != 60*time.Second {
		t.Errorf("environment client timeout = %s, want 60s", got)
	}
	if got := c.cluster.GetClient().Timeout; got != 60*time.Second {
		t.Errorf("cluster client timeout = %s, want 60s", got)
	}
}
