package main

import (
	"EasyConnectSuite/engine"
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestStopAndPowerLogsPreserveReasonAndConnectionState(t *testing.T) {
	a := multiTestApp(t)
	var buffer bytes.Buffer
	original := log.Writer()
	log.SetOutput(&buffer)
	defer log.SetOutput(original)
	start := func(o engine.Options) (vpnEngine, error) {
		if o.ConnectionID != "a" {
			t.Fatal("engine lost profile correlation")
		}
		e := &fakeVPNEngine{opts: o}
		o.OnStatus(engine.Status{Phase: engine.PhaseConnected})
		return e, nil
	}
	if result := a.startConnection(start); result != "OK" {
		t.Fatal(result)
	}
	for _, event := range []uintptr{4, 18, 7} {
		if !a.logPowerEvent(event) {
			t.Fatal("known power event ignored")
		}
	}
	if a.logPowerEvent(99) {
		t.Fatal("unrecognized notification must not be labeled suspend")
	}
	if !a.GetStatus().Running || a.GetStatus().State != "connected" {
		t.Fatal("logging changed the connection")
	}
	a.stopAllConnections("app_shutdown")
	text := buffer.String()
	for _, marker := range []string{`profile="a"`, "source=WM_POWERBROADCAST event=suspend", "event=resume_automatic", "event=resume_user", "stop requested reason=app_shutdown"} {
		if !strings.Contains(text, marker) {
			t.Fatal("missing stop/power evidence", marker)
		}
	}
}
