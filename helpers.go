package mut_ipc

// Helper functions for common message patterns

// BroadcastSimple sends a simple message with just type and ID
func (s *Server) BroadcastSimple(msgType, id string) {
	msg := Message{
		Type: msgType,
		ID:   id,
	}
	s.Broadcast(msg)
}

// BroadcastWithData sends a message with custom data payload
func (s *Server) BroadcastWithData(msgType, id string, data interface{}) {
	msg := Message{
		Type: msgType,
		ID:   id,
		Data: data,
	}
	s.Broadcast(msg)
}
