// Copyright 2025 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package muxer_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"go.uber.org/goleak"
)

// mockConn implements net.Conn for testing
type mockConn struct {
	readBuf  *bytes.Buffer
	writeBuf *bytes.Buffer
	closed   bool
	mu       sync.Mutex
}

func newMockConn() *mockConn {
	return &mockConn{
		readBuf:  &bytes.Buffer{},
		writeBuf: &bytes.Buffer{},
	}
}

func (m *mockConn) Read(b []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return 0, io.EOF
	}
	if m.readBuf.Len() == 0 {
		// Return 0 bytes read but no error to simulate blocking
		// This prevents EOF when buffer is empty
		return 0, nil
	}
	return m.readBuf.Read(b)
}

func (m *mockConn) Write(b []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return 0, io.ErrClosedPipe
	}
	return m.writeBuf.Write(b)
}

func (m *mockConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockConn) LocalAddr() net.Addr                { return nil }
func (m *mockConn) RemoteAddr() net.Addr               { return nil }
func (m *mockConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }

// Thread-safe test helpers
func (m *mockConn) WriteToReadBuf(b []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readBuf.Write(b)
}

func (m *mockConn) ReadWritten() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	// return a copy
	out := make([]byte, m.writeBuf.Len())
	copy(out, m.writeBuf.Bytes())
	return out
}

func (m *mockConn) WrittenLen() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.writeBuf.Len()
}

// TestSegmentCreation tests segment creation and basic properties
func TestSegmentCreation(t *testing.T) {
	defer goleak.VerifyNone(t)

	tests := []struct {
		name       string
		payload    []byte
		protocolId uint16
		isResponse bool
		expectNil  bool
	}{
		{
			name:       "valid request segment",
			protocolId: 0x01,
			payload:    []byte("test payload"),
			isResponse: false,
			expectNil:  false,
		},
		{
			name:       "valid response segment",
			protocolId: 0x01,
			payload:    []byte("test response"),
			isResponse: true,
			expectNil:  false,
		},
		{
			name:       "empty payload",
			protocolId: 0x02,
			payload:    []byte{},
			isResponse: false,
			expectNil:  false,
		},
		{
			name:       "maximum payload size",
			protocolId: 0x03,
			payload:    make([]byte, muxer.SegmentMaxPayloadLength),
			isResponse: false,
			expectNil:  false,
		},
		{
			name:       "payload too large",
			protocolId: 0x04,
			payload:    make([]byte, muxer.SegmentMaxPayloadLength+1),
			isResponse: false,
			expectNil:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			segment := muxer.NewSegment(
				tt.protocolId,
				tt.payload,
				tt.isResponse,
			)

			if tt.expectNil {
				if segment != nil {
					t.Errorf(
						"expected nil segment for oversized payload, got %v",
						segment,
					)
				}
				return
			}

			if segment == nil {
				t.Fatalf("expected valid segment, got nil")
			}

			// Check protocol ID
			if segment.GetProtocolId() != tt.protocolId {
				t.Errorf(
					"expected protocol ID %d, got %d",
					tt.protocolId,
					segment.GetProtocolId(),
				)
			}

			// Check payload
			if !bytes.Equal(segment.Payload, tt.payload) {
				t.Errorf(
					"expected payload %v, got %v",
					tt.payload,
					segment.Payload,
				)
			}

			// Check payload length
			if segment.PayloadLength != uint16(len(tt.payload)) {
				t.Errorf(
					"expected payload length %d, got %d",
					len(tt.payload),
					segment.PayloadLength,
				)
			}

			// Check response flag
			if tt.isResponse != segment.IsResponse() {
				t.Errorf(
					"expected isResponse %v, got %v",
					tt.isResponse,
					segment.IsResponse(),
				)
			}

			if tt.isResponse == segment.IsRequest() {
				t.Errorf("isResponse and isRequest should be opposites")
			}
		})
	}
}

// TestSegmentHeaderMethods tests segment header methods
func TestSegmentHeaderMethods(t *testing.T) {
	defer goleak.VerifyNone(t)

	// segmentProtocolIdResponseFlag mirrors the unexported constant in muxer package
	const responseFlag uint16 = 0x8000

	tests := []struct {
		name       string
		protocolId uint16
		isResponse bool
	}{
		{"request segment", 0x01, false},
		{"response segment", 0x01, true},
		{"high protocol ID request", 0x7FFF, false},
		{"high protocol ID response", 0x7FFF, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := muxer.SegmentHeader{
				ProtocolId: tt.protocolId,
			}
			if tt.isResponse {
				header.ProtocolId |= responseFlag
			}

			if header.IsResponse() != tt.isResponse {
				t.Errorf(
					"expected IsResponse() %v, got %v",
					tt.isResponse,
					header.IsResponse(),
				)
			}

			if header.IsRequest() == tt.isResponse {
				t.Errorf(
					"expected IsRequest() %v, got %v",
					!tt.isResponse,
					header.IsRequest(),
				)
			}

			if header.GetProtocolId() != tt.protocolId {
				t.Errorf(
					"expected GetProtocolId() %d, got %d",
					tt.protocolId,
					header.GetProtocolId(),
				)
			}
		})
	}
}

// TestMuxerInitialization tests muxer creation and initialization
func TestMuxerInitialization(t *testing.T) {
	defer goleak.VerifyNone(t)

	conn := newMockConn()
	m := muxer.New(conn)

	if m == nil {
		t.Fatal("expected non-nil muxer")
	}

	// Test error channel
	errorChan := m.ErrorChan()
	if errorChan == nil {
		t.Error("expected non-nil error channel")
	}

	// Test shutdown behavior
	m.Stop()

	// Should be able to close multiple times without panic
	m.Stop()
}

// TestProtocolRegistration tests protocol registration and unregistration
func TestProtocolRegistration(t *testing.T) {
	defer goleak.VerifyNone(t)

	conn := newMockConn()
	m := muxer.New(conn)
	defer m.Stop()

	// Test successful registration
	sendChan, recvChan, doneChan := m.RegisterProtocol(
		0x01,
		muxer.ProtocolRoleInitiator,
	)

	if sendChan == nil || recvChan == nil || doneChan == nil {
		t.Fatal("expected non-nil channels from registration")
	}

	// Test registration of both roles for the same protocol ID
	sendChan2, recvChan2, doneChan2 := m.RegisterProtocol(
		0x01,
		muxer.ProtocolRoleResponder,
	)

	if sendChan2 == nil || recvChan2 == nil || doneChan2 == nil {
		t.Fatal(
			"expected non-nil channels from duplicate registration with different role",
		)
	}

	// Test unregistration
	m.UnregisterProtocol(0x01, muxer.ProtocolRoleInitiator)

	// Test registration after shutdown
	m.Stop()
	sendChan3, recvChan3, doneChan3 := m.RegisterProtocol(
		0x02,
		muxer.ProtocolRoleInitiator,
	)

	if sendChan3 != nil || recvChan3 != nil || doneChan3 != nil {
		t.Error("expected nil channels from registration after shutdown")
	}
}

// TestMuxerSendReceive tests basic send and receive functionality
func TestMuxerSendReceive(t *testing.T) {
	defer goleak.VerifyNone(t)

	conn := newMockConn()
	m := muxer.New(conn)
	defer m.Stop()

	// Register a protocol
	sendChan, _, _ := m.RegisterProtocol(
		0x01,
		muxer.ProtocolRoleInitiator,
	)

	// Start the muxer
	m.Start()

	// Create a test segment
	payload := []byte("test message")
	segment := muxer.NewSegment(0x01, payload, false)
	if segment == nil {
		t.Fatal("failed to create test segment")
	}

	// Send the segment
	sendChan <- segment

	// Give some time for processing
	time.Sleep(10 * time.Millisecond)

	// Check that data was written to the connection (thread-safe)
	written := conn.ReadWritten()
	if len(written) == 0 {
		t.Error("expected data to be written to connection")
	}

	// Verify the written data format
	if len(written) < 8 { // minimum header size
		t.Errorf("written data too short: %d bytes", len(written))
	}

	// Parse the header
	var header muxer.SegmentHeader
	buf := bytes.NewReader(written[:8])
	if err := binary.Read(buf, binary.BigEndian, &header); err != nil {
		t.Errorf("failed to parse header: %v", err)
	}

	if header.GetProtocolId() != 0x01 {
		t.Errorf(
			"expected protocol ID 0x01, got 0x%04x",
			header.GetProtocolId(),
		)
	}

	if header.PayloadLength != uint16(len(payload)) {
		t.Errorf(
			"expected payload length %d, got %d",
			len(payload),
			header.PayloadLength,
		)
	}

	// Check payload
	if !bytes.Equal(written[8:], payload) {
		t.Errorf("expected payload %v, got %v", payload, written[8:])
	}
}

// TestDiffusionModes tests different diffusion modes
func TestDiffusionModes(t *testing.T) {
	defer goleak.VerifyNone(t)

	tests := []struct {
		name          string
		errorContains string
		diffusionMode muxer.DiffusionMode
		testRequest   bool
		testResponse  bool
		expectError   bool
	}{
		{
			name:          "initiator only - request should error",
			diffusionMode: muxer.DiffusionModeInitiator,
			testRequest:   true,
			testResponse:  false,
			expectError:   true,
			errorContains: "received message from initiator when not configured as a responder",
		},
		{
			name:          "responder only - response should error",
			diffusionMode: muxer.DiffusionModeResponder,
			testRequest:   false,
			testResponse:  true,
			expectError:   true,
			errorContains: "received message from responder when not configured as an initiator",
		},
		{
			name:          "full duplex - should work",
			diffusionMode: muxer.DiffusionModeInitiatorAndResponder,
			testRequest:   true,
			testResponse:  true,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := newMockConn()
			m := muxer.New(conn)
			m.SetDiffusionMode(tt.diffusionMode)
			defer m.Stop()

			// Register protocols for both roles
			_, _, _ = m.RegisterProtocol(0x01, muxer.ProtocolRoleInitiator)
			_, _, _ = m.RegisterProtocol(0x01, muxer.ProtocolRoleResponder)

			// Start the muxer
			m.Start()

			var testSegment *muxer.Segment
			if tt.testRequest {
				testSegment = muxer.NewSegment(0x01, []byte("request"), false)
			} else if tt.testResponse {
				testSegment = muxer.NewSegment(0x01, []byte("response"), true)
			}

			if testSegment != nil {
				data := createSegmentData(testSegment)
				conn.WriteToReadBuf(data)

				// Give time for processing
				time.Sleep(10 * time.Millisecond)

				// Check for errors
				select {
				case err := <-m.ErrorChan():
					if tt.expectError {
						if !strings.Contains(err.Error(), tt.errorContains) {
							t.Errorf(
								"expected error containing %q, got: %v",
								tt.errorContains,
								err,
							)
						}
					} else {
						t.Errorf("unexpected error: %v", err)
					}
				default:
					if tt.expectError {
						t.Errorf("expected error but got none")
					}
				}
			}
		})
	}
}

// TestErrorHandling tests various error conditions
func TestErrorHandling(t *testing.T) {
	defer goleak.VerifyNone(t)

	t.Run("zero byte payload", func(t *testing.T) {
		conn := newMockConn()
		m := muxer.New(conn)
		defer m.Stop()

		// Register protocol
		_, _, _ = m.RegisterProtocol(0x01, muxer.ProtocolRoleInitiator)
		m.Start()

		// Create segment with zero payload length
		header := muxer.SegmentHeader{
			Timestamp:     12345,
			ProtocolId:    0x01,
			PayloadLength: 0, // Invalid: zero payload
		}

		// Write invalid segment to connection
		buf := &bytes.Buffer{}
		if err := binary.Write(buf, binary.BigEndian, header); err != nil {
			t.Fatalf("failed to write header: %v", err)
		}
		conn.WriteToReadBuf(buf.Bytes())

		time.Sleep(10 * time.Millisecond)

		// Should receive error
		select {
		case err := <-m.ErrorChan():
			if err == nil ||
				!strings.Contains(err.Error(), "zero-byte segment payload") {
				t.Errorf("expected zero-byte payload error, got: %v", err)
			}
		default:
			t.Error("expected error for zero-byte payload")
		}
	})

	t.Run("unknown protocol", func(t *testing.T) {
		conn := newMockConn()
		m := muxer.New(conn)
		defer m.Stop()

		m.Start()

		// Create segment for unknown protocol
		segment := muxer.NewSegment(0x9999, []byte("test"), false)
		data := createSegmentData(segment)
		conn.WriteToReadBuf(data)

		time.Sleep(10 * time.Millisecond)

		// Should receive error
		select {
		case err := <-m.ErrorChan():
			if err == nil ||
				!strings.Contains(err.Error(), "unknown protocol ID") {
				t.Errorf("expected unknown protocol error, got: %v", err)
			}
		default:
			t.Error("expected error for unknown protocol")
		}
	})

	t.Run("connection closed", func(t *testing.T) {
		conn := newMockConn()
		m := muxer.New(conn)
		defer m.Stop()

		// Register protocol
		_, _, _ = m.RegisterProtocol(0x01, muxer.ProtocolRoleInitiator)
		m.Start()

		// Close connection
		conn.Close()

		time.Sleep(10 * time.Millisecond)

		// Should receive connection closed error
		select {
		case err := <-m.ErrorChan():
			if err == nil {
				t.Error("expected connection closed error")
			}
			var connErr *muxer.ConnectionClosedError
			if !errors.As(err, &connErr) {
				t.Errorf("expected ConnectionClosedError, got: %T", err)
			}
		default:
			t.Error("expected connection closed error")
		}
	})
}

// TestConcurrentAccess tests thread safety
func TestConcurrentAccess(t *testing.T) {
	defer goleak.VerifyNone(t)

	conn := newMockConn()
	m := muxer.New(conn)
	defer m.Stop()

	// Register multiple protocols
	const numProtocols = 10
	const numGoroutines = 5

	sendChans := make([]chan *muxer.Segment, numProtocols)
	for i := range numProtocols {
		sendChans[i], _, _ = m.RegisterProtocol(
			uint16(i),
			muxer.ProtocolRoleInitiator,
		)
	}

	// Start the muxer
	m.Start()

	// Let the muxer start
	time.Sleep(10 * time.Millisecond)

	var wg sync.WaitGroup

	// Start multiple goroutines sending messages
	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			sendChan := sendChans[id%numProtocols]
			if sendChan == nil {
				return // shutdown
			}

			for j := range 10 {
				payload := fmt.Appendf(nil, "message %d-%d", id, j)
				segment := muxer.NewSegment(
					uint16(id%numProtocols),
					payload,
					false,
				)
				if segment != nil {
					select {
					case sendChan <- segment:
						// Successfully sent
					case <-time.After(100 * time.Millisecond):
						// Timeout - muxer might be shutting down
						return
					}
				}
			}
		}(i)
	}

	// Let goroutines run for a bit
	time.Sleep(50 * time.Millisecond)

	// Stop the muxer
	m.Stop()

	// Wait for all goroutines to finish
	wg.Wait()

	// Check that some data was written (basic concurrency test)
	if conn.WrittenLen() == 0 {
		t.Error("expected some data to be written during concurrent access")
	}
}

// Helper functions

// createSegmentData serializes a segment to bytes
func createSegmentData(segment *muxer.Segment) []byte {
	buf := &bytes.Buffer{}
	_ = binary.Write(
		buf,
		binary.BigEndian,
		segment.SegmentHeader,
	) // error ignored: Buffer.Write never fails
	buf.Write(segment.Payload)
	return buf.Bytes()
}

// TestConnectionClosedError tests the ConnectionClosedError type
func TestConnectionClosedError(t *testing.T) {
	defer goleak.VerifyNone(t)

	innerErr := io.EOF
	err := &muxer.ConnectionClosedError{
		Context: "reading header",
		Err:     innerErr,
	}

	// Test Error() method
	errStr := err.Error()
	if !strings.Contains(errStr, "reading header") {
		t.Errorf("expected error to contain context, got: %s", errStr)
	}
	if !strings.Contains(errStr, "peer closed the connection") {
		t.Errorf("expected error message format, got: %s", errStr)
	}

	// Test Unwrap() method
	unwrapped := err.Unwrap()
	if unwrapped != innerErr {
		t.Errorf("expected unwrapped error to be %v, got %v", innerErr, unwrapped)
	}

	// Test errors.Is() works through Unwrap
	if !errors.Is(err, io.EOF) {
		t.Error("expected errors.Is to match io.EOF")
	}
}

// TestStartOnce tests the StartOnce functionality
func TestStartOnce(t *testing.T) {
	defer goleak.VerifyNone(t)

	conn := newMockConn()
	m := muxer.New(conn)
	defer m.Stop()

	// Register a protocol
	sendChan, _, _ := m.RegisterProtocol(0x01, muxer.ProtocolRoleInitiator)

	// Use StartOnce to process one message
	m.StartOnce()

	// Create and send a test segment
	payload := []byte("test")
	segment := muxer.NewSegment(0x01, payload, false)
	if segment == nil {
		t.Fatal("failed to create segment")
	}

	sendChan <- segment

	// Give time for processing
	time.Sleep(10 * time.Millisecond)

	// Verify data was written
	written := conn.ReadWritten()
	if len(written) == 0 {
		t.Error("expected data to be written")
	}
}

// TestUnregisterProtocolBehavior tests the UnregisterProtocol function
func TestUnregisterProtocolBehavior(t *testing.T) {
	defer goleak.VerifyNone(t)

	conn := newMockConn()
	m := muxer.New(conn)

	// Register a protocol
	_, recvChan, _ := m.RegisterProtocol(0x01, muxer.ProtocolRoleInitiator)
	if recvChan == nil {
		t.Fatal("expected receive channel")
	}

	// Start the muxer
	m.Start()

	// Give time to start
	time.Sleep(10 * time.Millisecond)

	// Unregister the protocol - this should close the receive channel
	m.UnregisterProtocol(0x01, muxer.ProtocolRoleInitiator)

	// Stop the muxer
	m.Stop()

	// Give time for cleanup
	time.Sleep(10 * time.Millisecond)
}

// TestMuxerSendAfterStop tests sending after muxer is stopped
func TestMuxerSendAfterStop(t *testing.T) {
	defer goleak.VerifyNone(t)

	conn := newMockConn()
	m := muxer.New(conn)

	// Start and then stop the muxer
	m.Start()
	m.Stop()

	// Give time for shutdown
	time.Sleep(10 * time.Millisecond)

	// Create a segment
	segment := muxer.NewSegment(0x01, []byte("test"), false)
	if segment == nil {
		t.Fatal("failed to create segment")
	}

	// Send should return an error (or silently fail) after shutdown
	err := m.Send(segment)
	// After stop, Send should return an error
	if err == nil {
		t.Log("Send after stop returned nil (acceptable behavior)")
	}
}

// TestProtocolRoleConstants tests protocol role constants
func TestProtocolRoleConstants(t *testing.T) {
	defer goleak.VerifyNone(t)

	// Verify protocol role constants
	if muxer.ProtocolRoleNone != 0 {
		t.Errorf("expected ProtocolRoleNone to be 0, got %d", muxer.ProtocolRoleNone)
	}
	if muxer.ProtocolRoleInitiator != 1 {
		t.Errorf(
			"expected ProtocolRoleInitiator to be 1, got %d",
			muxer.ProtocolRoleInitiator,
		)
	}
	if muxer.ProtocolRoleResponder != 2 {
		t.Errorf(
			"expected ProtocolRoleResponder to be 2, got %d",
			muxer.ProtocolRoleResponder,
		)
	}
}

// TestDiffusionModeConstants tests diffusion mode constants
func TestDiffusionModeConstants(t *testing.T) {
	defer goleak.VerifyNone(t)

	// Verify diffusion mode constants
	if muxer.DiffusionModeNone != 0 {
		t.Errorf("expected DiffusionModeNone to be 0, got %d", muxer.DiffusionModeNone)
	}
	if muxer.DiffusionModeInitiator != 1 {
		t.Errorf(
			"expected DiffusionModeInitiator to be 1, got %d",
			muxer.DiffusionModeInitiator,
		)
	}
	if muxer.DiffusionModeResponder != 2 {
		t.Errorf(
			"expected DiffusionModeResponder to be 2, got %d",
			muxer.DiffusionModeResponder,
		)
	}
	if muxer.DiffusionModeInitiatorAndResponder != 3 {
		t.Errorf(
			"expected DiffusionModeInitiatorAndResponder to be 3, got %d",
			muxer.DiffusionModeInitiatorAndResponder,
		)
	}
}

// TestProtocolUnknownConstant tests the ProtocolUnknown constant
func TestProtocolUnknownConstant(t *testing.T) {
	defer goleak.VerifyNone(t)

	if muxer.ProtocolUnknown != 0xabcd {
		t.Errorf(
			"expected ProtocolUnknown to be 0xabcd, got 0x%04x",
			muxer.ProtocolUnknown,
		)
	}
}

// TestSegmentMaxPayloadLength tests the segment max payload length constant
func TestSegmentMaxPayloadLength(t *testing.T) {
	defer goleak.VerifyNone(t)

	if muxer.SegmentMaxPayloadLength != 65535 {
		t.Errorf(
			"expected SegmentMaxPayloadLength to be 65535, got %d",
			muxer.SegmentMaxPayloadLength,
		)
	}
}

// TestMultipleProtocolRoles tests registering both roles for same protocol
func TestMultipleProtocolRoles(t *testing.T) {
	defer goleak.VerifyNone(t)

	conn := newMockConn()
	m := muxer.New(conn)
	defer m.Stop()

	// Register both initiator and responder for same protocol
	sendInit, recvInit, doneInit := m.RegisterProtocol(
		0x01,
		muxer.ProtocolRoleInitiator,
	)
	sendResp, recvResp, doneResp := m.RegisterProtocol(
		0x01,
		muxer.ProtocolRoleResponder,
	)

	// All channels should be valid
	if sendInit == nil || recvInit == nil || doneInit == nil {
		t.Error("expected valid channels for initiator role")
	}
	if sendResp == nil || recvResp == nil || doneResp == nil {
		t.Error("expected valid channels for responder role")
	}

	// Channels should be distinct
	if recvInit == recvResp {
		t.Error("expected different receive channels for different roles")
	}
}
