package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

// Message represents a generic IPC message that can carry any payload
type Message struct {
	Type      string      `json:"type"`
	ID        string      `json:"id,omitempty"`
	Data      interface{} `json:"data,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}

// MessageHandler defines the interface for handling received messages
type MessageHandler interface {
	HandleMessage(msg Message, client net.Conn) error
}

// MessageHandlerFunc is a function type that implements MessageHandler
type MessageHandlerFunc func(msg Message, client net.Conn) error

// HandleMessage implements the MessageHandler interface
func (f MessageHandlerFunc) HandleMessage(msg Message, client net.Conn) error {
	return f(msg, client)
}

// Broadcaster defines the interface for broadcasting messages
type Broadcaster interface {
	Broadcast(msg Message)
	Start() error
	Stop() error
}

// Server implements the IPC server functionality
type Server struct {
	socketPath      string
	listener        net.Listener
	clients         map[net.Conn]struct{}
	messageHandlers map[string]MessageHandler
	mu              sync.RWMutex
	stopChan        chan struct{}
	stopped         bool
}

// NewServer creates a new IPC server
func NewServer(socketPath string) *Server {
	return &Server{
		socketPath:      socketPath,
		clients:         make(map[net.Conn]struct{}),
		messageHandlers: make(map[string]MessageHandler),
		stopChan:        make(chan struct{}),
	}
}

// Start starts the IPC server
func (s *Server) Start() error {
	// Remove existing socket file if it exists
	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing socket: %w", err)
	}

	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket: %w", err)
	}

	s.listener = listener
	go s.acceptConnections()
	return nil
}

// Stop stops the IPC server
func (s *Server) Stop() error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return nil
	}
	s.stopped = true
	close(s.stopChan)
	s.mu.Unlock()

	if s.listener != nil {
		err := s.listener.Close()
		if err != nil {
			return err
		}
	}

	// Close all client connections
	s.mu.Lock()
	for conn := range s.clients {
		err := conn.Close()
		if err != nil {
			return err
		}
	}
	s.mu.Unlock()

	return os.Remove(s.socketPath)
}

// acceptConnections handles incoming client connections
func (s *Server) acceptConnections() {
	for {
		select {
		case <-s.stopChan:
			return
		default:
			conn, err := s.listener.Accept()
			if err != nil {
				select {
				case <-s.stopChan:
					return // Server is shutting down
				default:
					log.Printf("Error accepting connection: %v", err)
					continue
				}
			}

			s.mu.Lock()
			s.clients[conn] = struct{}{}
			s.mu.Unlock()

			log.Printf("Client connected: %s", conn.RemoteAddr())

			// Handle client messages and disconnection
			go func(c net.Conn) {
				defer func() {
					s.mu.Lock()
					delete(s.clients, c)
					s.mu.Unlock()
					err := c.Close()
					if err != nil {
						return
					}
				}()

				scanner := bufio.NewScanner(c)
				for scanner.Scan() {
					line := scanner.Text()
					if line == "" {
						continue // Skip empty lines
					}

					// Parse the JSON message
					var msg Message
					if err := json.Unmarshal([]byte(line), &msg); err != nil {
						log.Printf("Error parsing message from client %s: %v", c.RemoteAddr(), err)
						continue
					}

					// Find and execute the appropriate handler
					s.mu.RLock()
					handler, exists := s.messageHandlers[msg.Type]
					s.mu.RUnlock()

					if exists {
						if err := handler.HandleMessage(msg, c); err != nil {
							log.Printf("Error handling message type '%s' from client %s: %v", msg.Type, c.RemoteAddr(), err)
						}
					} else {
						log.Printf("No handler registered for message type '%s' from client %s", msg.Type, c.RemoteAddr())
					}
				}

				if err := scanner.Err(); err != nil {
					log.Printf("Client %s disconnected: %v", c.RemoteAddr(), err)
				} else {
					log.Printf("Client %s disconnected", c.RemoteAddr())
				}
			}(conn)
		}
	}
}

// Broadcast sends a message to all connected clients
func (s *Server) Broadcast(msg Message) {
	msg.Timestamp = time.Now()

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling IPC message: %v", err)
		return
	}

	data = append(data, '\n') // Add newline for easier parsing

	s.mu.RLock()
	clients := make([]net.Conn, 0, len(s.clients))
	for conn := range s.clients {
		clients = append(clients, conn)
	}
	s.mu.RUnlock()

	for _, conn := range clients {
		if _, err := conn.Write(data); err != nil {
			log.Printf("Error writing to client %s: %v", conn.RemoteAddr(), err)
			// Remove failed client
			s.mu.Lock()
			delete(s.clients, conn)
			s.mu.Unlock()
			err := conn.Close()
			if err != nil {
				return
			}
		}
	}
}

// RegisterMessageHandler registers a handler for a specific message type
func (s *Server) RegisterMessageHandler(messageType string, handler MessageHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messageHandlers[messageType] = handler
}

// UnregisterMessageHandler removes a handler for a specific message type
func (s *Server) UnregisterMessageHandler(messageType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.messageHandlers, messageType)
}

// GetClientCount returns the number of connected clients
func (s *Server) GetClientCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients)
}
