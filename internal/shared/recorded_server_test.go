package shared

import (
	"net"
	"testing"
)

// The takeover stops the process the port's health answer names, not the PID the
// record happens to hold: a recycled PID must never be signalled.
func TestRecordedServerToStopIsTheProcessThatAnswers(t *testing.T) {
	dir := isolate(t)
	port := fakeServer(t, 200, map[string]interface{}{"app": HealthAppMarker, "pid": 7777, "port": 0, "data": dir})

	live, stop := recordedServerToStop(Instance{PID: 4242, Port: port})
	if !stop || live.PID != 7777 {
		t.Fatalf("got %+v stop=%v, want the answering process 7777", live, stop)
	}
	if live.Port != port {
		t.Errorf("port = %d, want the recorded %d when the answer names none", live.Port, port)
	}
}

func TestRecordedServerToStopLeavesAProcessThatDoesNotAnswerAsModelsDB(t *testing.T) {
	isolate(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	if _, stop := recordedServerToStop(Instance{PID: 4242, Port: port}); stop {
		t.Error("a listener that is silent was treated as the recorded server")
	}

	foreign := fakeServer(t, 200, map[string]interface{}{"app": "something-else", "pid": 99})
	if _, stop := recordedServerToStop(Instance{PID: 4242, Port: foreign}); stop {
		t.Error("a server that is not ModelsDB was treated as the recorded server")
	}
}

func TestRecordedServerToStopLeavesAServerForAnotherDataDir(t *testing.T) {
	isolate(t)
	port := fakeServer(t, 200, map[string]interface{}{"app": HealthAppMarker, "pid": 7777, "data": "/somewhere/else"})
	if _, stop := recordedServerToStop(Instance{PID: 4242, Port: port}); stop {
		t.Error("a ModelsDB server for another data dir was stopped")
	}
}
