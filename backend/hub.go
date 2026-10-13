package main

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// Message is what the server sends to every browser.
type Message struct {
	Type  string   `json:"type"` // "message", "direct", "system" or "users"
	ID    string   `json:"id,omitempty"`
	User  string   `json:"user,omitempty"`  // who sent it
	To    string   `json:"to,omitempty"`    // recipient in a direct message
	Text  string   `json:"text,omitempty"`  // message text
	Users []string `json:"users,omitempty"` // online users (type "users")
	Time  string   `json:"time"`
}

type directMessage struct {
	sender    *Client
	recipient string
	text      string
}

type storedMessage struct {
	ID   primitive.ObjectID `bson:"_id,omitempty"`
	Type string             `bson:"type"`
	User string             `bson:"user"`
	To   string             `bson:"to,omitempty"`
	Text string             `bson:"text"`
	Time time.Time          `bson:"time"`
}

// Hub keeps all connected clients and sends each message to all of them.
// Only the run() goroutine touches the clients map, so no mutex is needed.
type Hub struct {
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	broadcast  chan Message
	direct     chan directMessage
	history    *mongo.Collection
}

func newHub(history ...*mongo.Collection) *Hub {
	hub := &Hub{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan Message),
		direct:     make(chan directMessage),
	}
	if len(history) > 0 {
		hub.history = history[0]
	}
	return hub
}

func encode(m Message) []byte {
	m.Time = time.Now().UTC().Format(time.RFC3339)
	data, _ := json.Marshal(m)
	return data
}

func (h *Hub) onlineUsers() []string {
	names := make([]string, 0, len(h.clients))
	seen := make(map[string]bool, len(h.clients))
	for c := range h.clients {
		if !seen[c.name] {
			names = append(names, c.name)
			seen[c.name] = true
		}
	}
	sort.Strings(names)
	return names
}

func (h *Hub) sendTo(c *Client, data []byte) bool {
	if !h.clients[c] {
		return false
	}
	select {
	case c.send <- data:
		return true
	default:
		close(c.send)
		delete(h.clients, c)
		return false
	}
}

func (h *Hub) saveMessage(message Message) (Message, error) {
	if h.history == nil {
		return message, nil
	}
	now := time.Now().UTC()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := h.history.InsertOne(ctx, storedMessage{
		Type: message.Type, User: message.User, To: message.To, Text: message.Text, Time: now,
	})
	if err != nil {
		return Message{}, err
	}
	id, ok := result.InsertedID.(primitive.ObjectID)
	if !ok {
		return Message{}, mongo.ErrNoDocuments
	}
	message.ID = id.Hex()
	message.Time = now.Format(time.RFC3339)
	return message, nil
}

func (h *Hub) deliverGroup(message Message) {
	saved, err := h.saveMessage(message)
	if err != nil {
		h.sendToByName(message.User, encode(Message{Type: "system", Text: "Your message could not be saved. Please try again."}))
		return
	}
	h.sendAll(encode(saved))
}

func (h *Hub) sendToByName(name string, data []byte) {
	for client := range h.clients {
		if client.name == name {
			h.sendTo(client, data)
		}
	}
}

func (h *Hub) deliverDirect(message directMessage) {
	if !h.clients[message.sender] {
		return
	}
	recipients := make([]*Client, 0)
	for client := range h.clients {
		if client.name == message.recipient {
			recipients = append(recipients, client)
		}
	}
	if message.recipient == "" || message.recipient == message.sender.name || len(recipients) == 0 {
		h.sendTo(message.sender, encode(Message{
			Type: "system",
			To:   message.recipient,
			Text: "That person is not online. Your message was not sent.",
		}))
		return
	}

	saved, err := h.saveMessage(Message{Type: "direct", User: message.sender.name, To: message.recipient, Text: message.text})
	if err != nil {
		h.sendTo(message.sender, encode(Message{
			Type: "system",
			To:   message.recipient,
			Text: "Your message could not be saved. Please try again.",
		}))
		return
	}

	delivered := false
	for _, recipient := range recipients {
		received := saved
		received.To = message.sender.name
		if h.sendTo(recipient, encode(received)) {
			delivered = true
		}
	}
	if !delivered {
		h.sendTo(message.sender, encode(Message{
			Type: "system",
			To:   message.recipient,
			Text: "That person is not online. Your message was not sent.",
		}))
		return
	}
	for client := range h.clients {
		if client.name == message.sender.name {
			h.sendTo(client, encode(saved))
		}
	}
}

// sendAll sends data to every client. A client with a full buffer is dropped.
func (h *Hub) sendAll(data []byte) {
	for c := range h.clients {
		select {
		case c.send <- data:
		default:
			close(c.send)
			delete(h.clients, c)
		}
	}
}

func (h *Hub) run() {
	for {
		select {
		case c := <-h.register:
			h.clients[c] = true
			h.sendAll(encode(Message{Type: "system", Text: c.name + " joined the chat"}))
			h.sendAll(encode(Message{Type: "users", Users: h.onlineUsers()}))

		case c := <-h.unregister:
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
				h.sendAll(encode(Message{Type: "system", Text: c.name + " left the chat"}))
				h.sendAll(encode(Message{Type: "users", Users: h.onlineUsers()}))
			}

		case message := <-h.broadcast:
			h.deliverGroup(message)

		case message := <-h.direct:
			h.deliverDirect(message)
		}
	}
}
