package ws

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
)

type Event struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

type Client struct {
	conn net.Conn
	send chan []byte
}

type Hub struct {
	mu         sync.RWMutex
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
}

var globalHub *Hub
var once sync.Once

func GetHub() *Hub {
	once.Do(func() {
		globalHub = &Hub{
			clients:    make(map[*Client]bool),
			broadcast:  make(chan []byte, 256),
			register:   make(chan *Client),
			unregister: make(chan *Client),
		}
		go globalHub.run()
	})
	return globalHub
}

func Broadcast(eventType string, payload interface{}) {
	h := GetHub()
	evt := Event{Type: eventType, Payload: payload}
	data, err := json.Marshal(evt)
	if err == nil {
		select {
		case h.broadcast <- data:
		default:
		}
	}
}

func (h *Hub) run() {
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
				_ = client.conn.Close()
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
					_ = client.conn.Close()
				}
			}
			h.mu.RUnlock()
		}
	}
}

// ServeWS handles WebSocket HTTP Handshake and upgrades connection via http.Hijacker
func ServeWS(w http.ResponseWriter, r *http.Request) {
	// Verify Token from header or query param
	token := r.URL.Query().Get("token")
	if token == "" {
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		}
	}

	if token == "" {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "missing Sec-WebSocket-Key", http.StatusBadRequest)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "webserver doesn't support hijacking", http.StatusInternalServerError)
		return
	}

	conn, bufrw, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Compute Sec-WebSocket-Accept key
	h := sha1.New()
	h.Write([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	acceptKey := base64.StdEncoding.EncodeToString(h.Sum(nil))

	// Send RFC 6455 Handshake Response
	response := fmt.Sprintf(
		"HTTP/1.1 101 Switching Protocols\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: Upgrade\r\n"+
			"Sec-WebSocket-Accept: %s\r\n\r\n", acceptKey,
	)
	if _, err := bufrw.WriteString(response); err != nil {
		conn.Close()
		return
	}
	_ = bufrw.Flush()

	client := &Client{
		conn: conn,
		send: make(chan []byte, 64),
	}

	hub := GetHub()
	hub.register <- client

	// Start writer goroutine
	go client.writePump(hub, bufrw.Writer)
	// Start reader goroutine
	go client.readPump(hub, bufrw.Reader)
}

func (c *Client) writePump(h *Hub, w *bufio.Writer) {
	defer func() {
		h.unregister <- c
	}()

	for msg := range c.send {
		// Encode text frame (0x81)
		length := len(msg)
		var frame []byte
		if length <= 125 {
			frame = []byte{0x81, byte(length)}
		} else if length <= 65535 {
			frame = make([]byte, 4)
			frame[0] = 0x81
			frame[1] = 126
			binary.BigEndian.PutUint16(frame[2:], uint16(length))
		} else {
			frame = make([]byte, 10)
			frame[0] = 0x81
			frame[1] = 127
			binary.BigEndian.PutUint64(frame[2:], uint64(length))
		}

		frame = append(frame, msg...)
		if _, err := w.Write(frame); err != nil {
			return
		}
		if err := w.Flush(); err != nil {
			return
		}
	}
}

func (c *Client) readPump(h *Hub, r *bufio.Reader) {
	defer func() {
		h.unregister <- c
	}()

	for {
		header := make([]byte, 2)
		if _, err := io.ReadFull(r, header); err != nil {
			return
		}
		opcode := header[0] & 0x0f
		if opcode == 0x08 { // Close frame
			return
		}
		masked := (header[1] & 0x80) != 0
		payloadLen := int(header[1] & 0x7f)

		if payloadLen == 126 {
			var l uint16
			if err := binary.Read(r, binary.BigEndian, &l); err != nil {
				return
			}
			payloadLen = int(l)
		} else if payloadLen == 127 {
			var l uint64
			if err := binary.Read(r, binary.BigEndian, &l); err != nil {
				return
			}
			payloadLen = int(l)
		}

		var maskKey []byte
		if masked {
			maskKey = make([]byte, 4)
			if _, err := io.ReadFull(r, maskKey); err != nil {
				return
			}
		}

		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(r, payload); err != nil {
			return
		}

		if masked {
			for i := 0; i < payloadLen; i++ {
				payload[i] ^= maskKey[i%4]
			}
		}
	}
}

func AuthWS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenStr := r.URL.Query().Get("token")
		if tokenStr == "" {
			tokenStr = r.Header.Get("Authorization")
			if len(tokenStr) > 7 && tokenStr[:7] == "Bearer " {
				tokenStr = tokenStr[7:]
			}
		}

		if tokenStr == "" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}
