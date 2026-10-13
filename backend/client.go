package main

import (
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 2048
	maxTextLength  = 500
	maxNameLength  = 20
)

// Client is one connected browser tab.
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte // messages waiting to be written to this browser
	name string
}

// incoming is the JSON the browser sends: {"text":"hello"}
type incoming struct {
	Text string `json:"text"`
	To   string `json:"to,omitempty"`
}

func cleanName(name string) string {
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) > maxNameLength {
		name = string([]rune(name)[:maxNameLength])
	}
	return name
}

// readPump reads messages from the browser and gives them to the hub.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var in incoming
		if json.Unmarshal(raw, &in) != nil {
			continue
		}
		text := strings.TrimSpace(in.Text)
		if text == "" {
			continue
		}
		if utf8.RuneCountInString(text) > maxTextLength {
			text = string([]rune(text)[:maxTextLength])
		}
		// The server sets the sender name, so nobody can pretend to be someone else.
		if in.To == "" {
			c.hub.broadcast <- Message{Type: "message", User: c.name, Text: text}
		} else {
			c.hub.direct <- directMessage{sender: c, recipient: strings.TrimSpace(in.To), text: text}
		}
	}
}

// writePump sends messages from the hub to the browser and pings it regularly.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case data, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
