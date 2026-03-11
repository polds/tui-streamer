package session

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
)

// Client wraps a single WebSocket connection subscribed to a Session.
type Client struct {
	conn    *websocket.Conn
	session *Session
	sendCh  chan []byte
	done    chan struct{}
	once    sync.Once
}

// NewClient creates a Client associated with the given session.
func NewClient(conn *websocket.Conn, sess *Session) *Client {
	return &Client{
		conn:    conn,
		session: sess,
		sendCh:  make(chan []byte, 256),
		done:    make(chan struct{}),
	}
}

// Run registers the client with its session then blocks until the connection
// closes. It is safe to call in a goroutine.
func (c *Client) Run() {
	c.session.subscribe(c)
	go c.writePump()
	c.readPump()
}

// Send enqueues msg for delivery. It is non-blocking and drops messages if the
// client's send buffer is full or the client has already disconnected.
func (c *Client) Send(msg []byte) {
	select {
	case c.sendCh <- msg:
	case <-c.done:
	default:
		// buffer full – drop rather than block the broadcaster
	}
}

// close tears down the client exactly once: unsubscribes from the session,
// closes the underlying connection, and signals the done channel.
func (c *Client) close() {
	c.once.Do(func() {
		c.session.unsubscribe(c)
		c.conn.Close()
		close(c.done)
	})
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.close()
	}()

	for {
		select {
		case msg, ok := <-c.sendCh:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}

		case <-c.done:
			return
		}
	}
}

func (c *Client) readPump() {
	defer c.close()

	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}
