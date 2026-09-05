// Package wsq is a small topic-based WebSocket hub: submission status and
// contest standings updates are published server-side and fanned out to
// subscribed browsers. Topics are plain strings, e.g. "submission:42",
// "contest:7", "admin:daemons".
package wsq

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 50 * time.Second
	maxClients = 1024
)

type message struct {
	Topic string `json:"topic"`
	Data  any    `json:"data"`
}

type client struct {
	conn   *websocket.Conn
	role   string
	userID uint
	topics map[string]struct{}
	mu     sync.Mutex
}

// Hub owns the connection set; zero value is ready after New.
type Hub struct {
	mu       sync.RWMutex
	clients  map[*client]struct{}
	upgrader websocket.Upgrader
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[*client]struct{}),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 4096,
			CheckOrigin: func(r *http.Request) bool {
				return true // same-campus SPA; tighten origins at the reverse proxy
			},
		},
	}
}

// TopicAuthorizer decides whether a connection may subscribe to a topic;
// infrastructure topics (admin:*) must not leak to ordinary users and
// per-submission topics are owner-only. The userID comes from the
// authenticated claims captured at upgrade time (0 = anonymous).
type TopicAuthorizer func(role string, userID uint, topic string) bool

// Handler returns the gin handler upgrading GET /api/v1/ws. The authorizer
// is captured per connection together with the authenticated identity.
func (h *Hub) Handler(authorize TopicAuthorizer) gin.HandlerFunc {
	return func(c *gin.Context) {
		if h.size() >= maxClients {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		role, _ := c.Get("auth.role")
		roleStr, _ := role.(string)
		id, _ := c.Get("auth.id")
		idNum, _ := id.(uint)
		cl := &client{conn: conn, role: roleStr, userID: idNum, topics: map[string]struct{}{}}
		h.mu.Lock()
		h.clients[cl] = struct{}{}
		h.mu.Unlock()
		go h.readLoop(cl, authorize)
		h.writeLoop(cl)
	}
}

func (h *Hub) size() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// readLoop processes subscribe/unsubscribe control frames until the peer
// closes; topic subscriptions are checked against the per-connection role.
func (h *Hub) readLoop(cl *client, authorize TopicAuthorizer) {
	defer h.remove(cl)
	_ = cl.conn.SetReadDeadline(time.Now().Add(pongWait))
	cl.conn.SetPongHandler(func(string) error {
		return cl.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		var req struct {
			Subscribe   string `json:"subscribe"`
			Unsubscribe string `json:"unsubscribe"`
		}
		if err := cl.conn.ReadJSON(&req); err != nil {
			return
		}
		cl.mu.Lock()
		if req.Subscribe != "" && authorize(cl.role, cl.userID, req.Subscribe) {
			cl.topics[req.Subscribe] = struct{}{}
		}
		if req.Unsubscribe != "" {
			delete(cl.topics, req.Unsubscribe)
		}
		cl.mu.Unlock()
	}
}

// writeLoop is the single writer per connection (gorilla requires it):
// it pings on schedule and pushes published messages.
func (h *Hub) writeLoop(cl *client) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = cl.conn.Close()
		h.remove(cl)
	}()
	events := make(chan message, 64)
	h.subscribeEvents(cl, events)
	for {
		select {
		case msg := <-events:
			_ = cl.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := cl.conn.WriteJSON(msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = cl.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := cl.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// eventRegistry maps clients to their event channels; kept separate from the
// topic map so Publish never blocks on a slow consumer (drops instead).
var eventRegistry = struct {
	mu sync.RWMutex
	m  map[*client]chan message
}{m: map[*client]chan message{}}

func (h *Hub) subscribeEvents(cl *client, events chan message) {
	eventRegistry.mu.Lock()
	defer eventRegistry.mu.Unlock()
	eventRegistry.m[cl] = events
}

func (h *Hub) remove(cl *client) {
	h.mu.Lock()
	delete(h.clients, cl)
	h.mu.Unlock()
	eventRegistry.mu.Lock()
	if ch, ok := eventRegistry.m[cl]; ok {
		close(ch)
		delete(eventRegistry.m, cl)
	}
	eventRegistry.mu.Unlock()
}

// Publish fans msg out to every client subscribed to the topic. Slow
// consumers drop messages rather than stall the judge pipeline; the SPA
// refetches on any gap, so at-least-once-plus-refetch is sufficient here.
func (h *Hub) Publish(topic string, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	eventRegistry.mu.RLock()
	defer eventRegistry.mu.RUnlock()
	for cl := range h.clients {
		cl.mu.Lock()
		_, subscribed := cl.topics[topic]
		cl.mu.Unlock()
		if !subscribed {
			continue
		}
		if ch, ok := eventRegistry.m[cl]; ok {
			select {
			case ch <- message{Topic: topic, Data: json.RawMessage(raw)}:
			default:
			}
		}
	}
}
