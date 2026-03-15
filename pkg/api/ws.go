package api

import (
    "context"
    "encoding/json"
    "log"
    "net/http"
    "os"
    "strings"
    "sync"

    "github.com/gorilla/websocket"
    "github.com/openexec/openexec/internal/loop"
    "github.com/openexec/openexec/internal/mcp"
    "github.com/openexec/openexec/internal/prompt"
)

var upgrader = websocket.Upgrader{
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
    CheckOrigin: func(r *http.Request) bool {
        // Allow only configured origins; default to localhost
        origin := r.Header.Get("Origin")
        if origin == "" || origin == "null" {
            // CLI/webviews without Origin
            return true
        }
        allowed := os.Getenv("OPENEXEC_WS_ALLOWED_ORIGINS")
        if allowed != "" {
            parts := strings.Split(allowed, ",")
            for _, a := range parts {
                a = strings.TrimSpace(a)
                if a != "" && strings.Contains(origin, a) {
                    return true
                }
            }
            return false
        }
        // Default allow: localhost and loopback
        if strings.Contains(origin, "localhost") || strings.Contains(origin, "127.0.0.1") || strings.Contains(origin, "[::1]") {
            return true
        }
        return false
    },
}

// Hub maintains the set of active clients and broadcasts messages to them.
type Hub struct {
	// Registered clients.
	clients map[*Client]bool

	// Inbound messages from the clients.
	broadcast chan []byte

	// Register requests from the clients.
	register chan *Client

	// Unregister requests from clients.
	unregister chan *Client

	mu sync.RWMutex
}

func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends a message to all clients.
func (h *Hub) Broadcast(v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("error marshaling broadcast message: %v", err)
		return
	}
	h.broadcast <- data
}

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub *Hub

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte

	// Active unsubscription function for current session
	unsub func()
}

func (c *Client) readPump(s *Server) {
	defer func() {
		if c.unsub != nil {
			c.unsub()
		}
		c.hub.unregister <- c
		c.conn.Close()
	}()
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[WS] error: %v", err)
			}
			break
		}

		var req struct {
			Type      string `json:"type"`
			SessionID string `json:"sessionId"`
			RunID     string `json:"runId"`
			Payload   any    `json:"payload"`
		}
		if err := json.Unmarshal(message, &req); err != nil {
			log.Printf("[WS] error unmarshaling message: %v", err)
			continue
		}

		switch req.Type {
		case "subscribe":
			if c.unsub != nil {
				c.unsub()
			}

			events, unsub, err := s.Mgr.Subscribe(req.SessionID)
			if err != nil {
				log.Printf("[WS] failed to subscribe to %s: %v", req.SessionID, err)
				continue
			}

			c.unsub = unsub
			log.Printf("[WS] Client subscribed to session: %s", req.SessionID)

			// Start event relay for this session
			go func(sessionId string, ch <-chan loop.Event) {
				for event := range ch {
					msg := map[string]interface{}{
						"type":                       "event",
						"sessionId":                  sessionId,
						"payload":                    event,
						"prompt_version":             prompt.PromptVersion,
						"tool_registry_version":      mcp.ToolRegistryVersion,
						"run_state_machine_version": prompt.RunStateMachineVersion,
					}
					data, _ := json.Marshal(msg)
					select {
					case c.send <- data:
					default:
						// Client slow or disconnected
						return
					}
				}
			}(req.SessionID, events)

		case "unsubscribe":
			if c.unsub != nil {
				c.unsub()
				c.unsub = nil
			}
		default:
			// Log and notify unknown message types to help client migration
			log.Printf("[WS] unknown message type: %s", req.Type)
			note := map[string]interface{}{"type": "notice", "message": "unknown request type; use subscribe, subscribe_steps, unsubscribe, pause, resume, stop, or ping"}
			if data, err := json.Marshal(note); err == nil {
				select { case c.send <- data: default: }
			}
		case "subscribe_steps":
			// For now, reuse the same event stream by run/session id. Future: dedicated run.step stream.
			if c.unsub != nil { c.unsub() }
			events, unsub, err := s.Mgr.Subscribe(req.RunID)
			if err != nil {
				log.Printf("[WS] failed to subscribe to steps for %s: %v", req.RunID, err)
				continue
			}
			c.unsub = unsub
			log.Printf("[WS] Client subscribed to steps: %s", req.RunID)
			go func(runID string, ch <-chan loop.Event) {
				for event := range ch {
					msg := map[string]interface{}{
						"type":                       "step",
						"runId":                      runID,
						"payload":                    event,
						"prompt_version":             prompt.PromptVersion,
						"tool_registry_version":      mcp.ToolRegistryVersion,
						"run_state_machine_version": prompt.RunStateMachineVersion,
					}
					data, _ := json.Marshal(msg)
					select {
					case c.send <- data:
					default:
						return
					}
				}
			}(req.RunID, events)

		case "pause":
			_ = s.Mgr.Pause(req.SessionID)
		case "resume":
			// resume currently not in manager, but start acts as resume if already created
			_ = s.Mgr.Start(context.Background(), req.SessionID)
		case "stop":
			_ = s.Mgr.Stop(req.SessionID)
		case "ping":
			c.send <- []byte(`{"type":"pong"}`)
		}
	}
}

func (c *Client) writePump() {
	defer func() {
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			if err := w.Close(); err != nil {
				return
			}
		}
	}
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] error upgrading: %v", err)
		return
	}
	client := &Client{hub: s.Hub, conn: conn, send: make(chan []byte, 256)}
	client.hub.register <- client

	go client.writePump()
	go client.readPump(s)
}
