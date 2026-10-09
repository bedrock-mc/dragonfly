package session

import (
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"testing"
)

// queuedDisconnectConn implements the existing queued-write connection contract.
type queuedDisconnectConn struct {
	Conn
	packets []packet.Packet
	flushes int
}

// WritePacket records packets queued for a subsequent flush.
func (c *queuedDisconnectConn) WritePacket(pk packet.Packet) error {
	c.packets = append(c.packets, pk)
	return nil
}

// Flush records submission of the pending batch.
func (c *queuedDisconnectConn) Flush() error { c.flushes++; return nil }

// immediateDisconnectConn adds immediate submission to the queued contract.
type immediateDisconnectConn struct {
	queuedDisconnectConn
	submissions int
}

// WritePacketImmediate records a batch submitted without a separate flush.
func (c *immediateDisconnectConn) WritePacketImmediate(pks ...packet.Packet) error {
	c.packets = append(c.packets, pks...)
	c.submissions++
	return nil
}

func TestDisconnectSupportsQueuedAndImmediateConnections(t *testing.T) {
	legacy := &queuedDisconnectConn{}
	(&Session{conn: legacy}).Disconnect("Goodbye")
	if len(legacy.packets) != 1 || legacy.flushes != 1 {
		t.Fatalf("queued disconnect not submitted: %+v", legacy)
	}
	pk := legacy.packets[0].(*packet.Disconnect)
	if pk.Message != "Goodbye" || pk.HideDisconnectionScreen {
		t.Fatalf("disconnect payload changed: %+v", pk)
	}
	modern := &immediateDisconnectConn{}
	(&Session{conn: modern}).Disconnect("")
	if len(modern.packets) != 1 || modern.submissions != 1 || modern.flushes != 0 {
		t.Fatalf("immediate disconnect used wrong submission path: %+v", modern)
	}
	if !modern.packets[0].(*packet.Disconnect).HideDisconnectionScreen {
		t.Fatal("empty disconnect reason must hide its screen")
	}
}
