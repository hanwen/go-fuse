package fuse

import "testing"

func TestMonitorOpcodeRegistered(t *testing.T) {
	handler := getHandler(_OP_MONITOR)
	if handler == nil {
		t.Fatal("expected monitor opcode handler to be registered")
	}
	if handler.Name != "MONITOR" {
		t.Fatalf("unexpected monitor opcode name %q", handler.Name)
	}
	if handler.Func == nil {
		t.Fatal("expected monitor opcode handler function")
	}
}
