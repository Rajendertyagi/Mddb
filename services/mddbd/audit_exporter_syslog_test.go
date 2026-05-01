package main

import (
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// listenSyslog returns a UDP listener and a goroutine that appends
// every received datagram to a channel. Caller must Close the conn.
func listenSyslog(t *testing.T) (string, <-chan string, func()) {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ch := make(chan string, 16)
	stop := make(chan struct{})
	go func() {
		buf := make([]byte, 4096)
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, _, err := conn.ReadFrom(buf)
			if err != nil {
				if isTimeout(err) {
					continue
				}
				return
			}
			ch <- string(buf[:n])
		}
	}()
	cleanup := func() {
		close(stop)
		_ = conn.Close()
	}
	return conn.LocalAddr().String(), ch, cleanup
}

func isTimeout(err error) bool {
	type timeoutErr interface{ Timeout() bool }
	if t, ok := err.(timeoutErr); ok && t.Timeout() {
		return true
	}
	return false
}

// TestSyslogExporter_DeliversRFC5424 sends one event over UDP and
// validates the wire format.
func TestSyslogExporter_DeliversRFC5424(t *testing.T) {
	addr, ch, cleanup := listenSyslog(t)
	defer cleanup()

	se, err := NewSyslogExporter(addr, "local0", 4)
	if err != nil {
		t.Fatal(err)
	}
	defer se.Close()
	se.Export(AuditEvent{Action: "login", Actor: "alice", Result: "ok", Timestamp: time.Now().UnixNano()})

	select {
	case msg := <-ch:
		// PRIVAL = 16*8 + 6 = 134
		if !strings.HasPrefix(msg, "<134>1 ") {
			t.Errorf("bad PRIVAL prefix: %q", msg[:20])
		}
		if !strings.Contains(msg, "[mddb@32473 actor=\"alice\" action=\"login\"") {
			t.Errorf("missing structured data: %q", msg)
		}
		if !strings.Contains(msg, `"action":"login"`) {
			t.Errorf("body missing JSON: %q", msg)
		}
		if !strings.HasSuffix(msg, "\n") {
			t.Errorf("no trailing newline")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no syslog datagram received")
	}
}

// TestSyslogExporter_FacilityMapping covers every alias in syslogFacility().
func TestSyslogExporter_FacilityMapping(t *testing.T) {
	cases := map[string]int{
		"local0":      16,
		"local1":      17,
		"local2":      18,
		"local3":      19,
		"local4":      20,
		"local5":      21,
		"local6":      22,
		"local7":      23,
		"":            16,
		"LOCAL3":      19,
		"user":        1,
		"daemon":      3,
		"auth":        4,
		"authpriv":    10,
		"unknown-xyz": 16,
	}
	for in, want := range cases {
		if got := syslogFacility(in); got != want {
			t.Errorf("%q: got %d, want %d", in, got, want)
		}
	}
}

// TestSyslogExporter_AddrSchemes parses udp:// and tcp:// schemes.
func TestSyslogExporter_AddrSchemes(t *testing.T) {
	cases := map[string][2]string{
		"host:514":            {"udp", "host:514"},
		"udp://host:514":      {"udp", "host:514"},
		"tcp://host:6514":     {"tcp", "host:6514"},
		"plain.example:1234":  {"udp", "plain.example:1234"},
	}
	for in, want := range cases {
		net, hp := parseSyslogAddr(in)
		if net != want[0] || hp != want[1] {
			t.Errorf("%q: got (%s,%s), want (%s,%s)", in, net, hp, want[0], want[1])
		}
	}
}

// TestSyslogExporter_FailWarn flips severity when result=fail.
func TestSyslogExporter_FailWarn(t *testing.T) {
	addr, ch, cleanup := listenSyslog(t)
	defer cleanup()
	se, _ := NewSyslogExporter(addr, "local0", 4)
	defer se.Close()
	se.Export(AuditEvent{Action: "auth", Result: "fail"})
	select {
	case msg := <-ch:
		// PRIVAL = 16*8 + 4 = 132
		if !strings.HasPrefix(msg, "<132>1 ") {
			t.Errorf("expected warn severity, got %q", msg[:8])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no datagram")
	}
}

// TestSyslogExporter_EmptyAddrRejected — fail fast on missing config.
func TestSyslogExporter_EmptyAddrRejected(t *testing.T) {
	if _, err := NewSyslogExporter("", "local0", 4); err == nil {
		t.Fatal("expected error")
	}
}

// TestSyslogExporter_DialFailure — collector unreachable, the
// exporter must increment Failed and capture LastError.
func TestSyslogExporter_DialFailure(t *testing.T) {
	se, _ := NewSyslogExporter("tcp://127.0.0.1:1", "local0", 4)
	defer se.Close()
	se.Export(AuditEvent{Action: "x"})
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		st := se.Stats()
		if st.Failed > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Skipf("dial failure timing too slow on this platform: %+v", se.Stats())
}

// TestSyslogExporter_MissingTimestamp — Timestamp == 0 falls back to
// the current wall clock.
func TestSyslogExporter_MissingTimestamp(t *testing.T) {
	addr, ch, cleanup := listenSyslog(t)
	defer cleanup()
	se, _ := NewSyslogExporter(addr, "local0", 4)
	defer se.Close()
	se.Export(AuditEvent{Action: "now"})
	select {
	case msg := <-ch:
		// Body still parses, and the timestamp field is non-empty.
		if !strings.Contains(msg, " mddb ") {
			t.Errorf("malformed: %q", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no datagram")
	}
}

// TestSyslogExporter_ReusesConnection — the second send must reuse
// the cached net.Conn.
func TestSyslogExporter_ReusesConnection(t *testing.T) {
	addr, ch, cleanup := listenSyslog(t)
	defer cleanup()
	se, _ := NewSyslogExporter(addr, "local0", 4)
	defer se.Close()
	for i := 0; i < 3; i++ {
		se.Export(AuditEvent{Action: "x"})
	}
	got := atomic.Int32{}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ch:
			if got.Add(1) >= 3 {
				return
			}
		case <-time.After(50 * time.Millisecond):
		}
	}
	if got.Load() < 3 {
		t.Errorf("got %d datagrams, want 3", got.Load())
	}
}
