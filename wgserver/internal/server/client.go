package server

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Client struct {
	ID       string
	Conn     *websocket.Conn
	Send     chan []byte
	Zone     string
	LastHBAt time.Time
	mu       sync.Mutex
}

func (c *Client) SafeWrite(msg []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Conn.WriteMessage(websocket.TextMessage, msg)
}
