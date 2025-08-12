package ipc

import (
	"encoding/json"
	"net"
	"os"
	"sync"
	"testing"
	"time"
)

func TestReceiveMessages(t *testing.T) {
	socketPath := "/tmp/test_ipc_receive.sock"
	
	// Clean up any existing socket
	os.Remove(socketPath)
	defer os.Remove(socketPath)

	// Create and start server
	server := NewServer(socketPath)
	
	// Track received messages
	var receivedMessages []Message
	var mu sync.Mutex
	
	// Register a message handler
	server.RegisterMessageHandler("test", MessageHandlerFunc(func(msg Message, client net.Conn) error {
		mu.Lock()
		receivedMessages = append(receivedMessages, msg)
		mu.Unlock()
		
		// Echo the message back to client
		response := Message{
			Type: "response",
			ID:   msg.ID,
			Data: map[string]interface{}{
				"original": msg.Data,
				"echo":     true,
			},
		}
		
		data, _ := json.Marshal(response)
		data = append(data, '\n')
		_, err := client.Write(data)
		return err
	}))

	// Start the server
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Connect as a client
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()

	// Send a test message
	testMsg := Message{
		Type: "test",
		ID:   "test-123",
		Data: map[string]interface{}{
			"content": "Hello from client",
			"number":  42,
		},
	}
	
	data, err := json.Marshal(testMsg)
	if err != nil {
		t.Fatalf("Failed to marshal test message: %v", err)
	}
	
	data = append(data, '\n')
	if _, err := conn.Write(data); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Give time for message processing
	time.Sleep(100 * time.Millisecond)

	// Check that message was received
	mu.Lock()
	if len(receivedMessages) != 1 {
		mu.Unlock()
		t.Fatalf("Expected 1 received message, got %d", len(receivedMessages))
	}
	
	received := receivedMessages[0]
	mu.Unlock()

	if received.Type != "test" {
		t.Errorf("Expected message type 'test', got '%s'", received.Type)
	}
	
	if received.ID != "test-123" {
		t.Errorf("Expected message ID 'test-123', got '%s'", received.ID)
	}

	// Verify data content
	dataMap, ok := received.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data to be a map, got %T", received.Data)
	}
	
	if dataMap["content"] != "Hello from client" {
		t.Errorf("Expected content 'Hello from client', got '%v'", dataMap["content"])
	}
	
	if dataMap["number"] != float64(42) { // JSON unmarshals numbers as float64
		t.Errorf("Expected number 42, got %v", dataMap["number"])
	}

	t.Logf("Successfully received and processed message: %+v", received)
}

func TestUnregisteredMessageType(t *testing.T) {
	socketPath := "/tmp/test_ipc_unregistered.sock"
	
	// Clean up any existing socket
	os.Remove(socketPath)
	defer os.Remove(socketPath)

	// Create and start server without registering any handlers
	server := NewServer(socketPath)
	
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Connect as a client
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()

	// Send a message with unregistered type
	testMsg := Message{
		Type: "unregistered",
		ID:   "test-456",
		Data: "test data",
	}
	
	data, err := json.Marshal(testMsg)
	if err != nil {
		t.Fatalf("Failed to marshal test message: %v", err)
	}
	
	data = append(data, '\n')
	if _, err := conn.Write(data); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Give time for message processing
	time.Sleep(100 * time.Millisecond)

	// The test passes if no panic occurs - the server should handle
	// unregistered message types gracefully by logging and continuing
	t.Log("Server handled unregistered message type gracefully")
}