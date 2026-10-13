package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestDeliverDirectOnlySendsToConversationMembers(t *testing.T) {
	hub := newHub()
	alice := &Client{name: "Alice", send: make(chan []byte, 1)}
	aliceSecondTab := &Client{name: "Alice", send: make(chan []byte, 1)}
	bob := &Client{name: "Bob", send: make(chan []byte, 1)}
	carol := &Client{name: "Carol", send: make(chan []byte, 1)}
	hub.clients[alice] = true
	hub.clients[aliceSecondTab] = true
	hub.clients[bob] = true
	hub.clients[carol] = true

	hub.deliverDirect(directMessage{sender: alice, recipient: "Bob", text: "private hello"})

	assertDirectMessage := func(client *Client, wantUser, wantTo string) {
		t.Helper()
		select {
		case raw := <-client.send:
			var message Message
			if err := json.Unmarshal(raw, &message); err != nil {
				t.Fatalf("decode message: %v", err)
			}
			if message.Type != "direct" || message.User != wantUser || message.To != wantTo || message.Text != "private hello" {
				t.Errorf("direct message = %+v, want user %q to %q", message, wantUser, wantTo)
			}
		default:
			t.Errorf("%s did not receive a direct message", client.name)
		}
	}
	assertDirectMessage(alice, "Alice", "Bob")
	assertDirectMessage(aliceSecondTab, "Alice", "Bob")
	assertDirectMessage(bob, "Alice", "Alice")
	select {
	case message := <-carol.send:
		t.Errorf("unrelated user received a private message: %s", message)
	default:
	}
}

func TestDeliverDirectReportsOfflineRecipientOnlyToSender(t *testing.T) {
	hub := newHub()
	alice := &Client{name: "Alice", send: make(chan []byte, 1)}
	carol := &Client{name: "Carol", send: make(chan []byte, 1)}
	hub.clients[alice] = true
	hub.clients[carol] = true

	hub.deliverDirect(directMessage{sender: alice, recipient: "Bob", text: "private hello"})

	select {
	case raw := <-alice.send:
		var message Message
		if err := json.Unmarshal(raw, &message); err != nil {
			t.Fatalf("decode offline notification: %v", err)
		}
		if message.Type != "system" || message.To != "Bob" {
			t.Errorf("offline notification = %+v, want system notice for Bob", message)
		}
	default:
		t.Fatal("sender did not receive an offline notification")
	}
	select {
	case message := <-carol.send:
		t.Errorf("unrelated user received an offline notification: %s", message)
	default:
	}
}

func TestWebSocketDirectMessageDoesNotReachOtherUsers(t *testing.T) {
	hub := newHub()
	go hub.run()
	auth := &authService{signingKey: []byte("websocket-test-signing-key-with-32-bytes")}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveWS(hub, auth, w, r)
	}))
	defer server.Close()
	wsURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	wsURL.Scheme = "ws"
	wsURL.Path = "/ws"

	connect := func(name string) *websocket.Conn {
		t.Helper()
		token, err := auth.issueToken(name)
		if err != nil {
			t.Fatalf("issue %s token: %v", name, err)
		}
		conn, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
		if err != nil {
			t.Fatalf("connect %s: %v", name, err)
		}
		if err := conn.WriteJSON(map[string]string{"type": "auth", "token": token}); err != nil {
			conn.Close()
			t.Fatalf("authenticate %s: %v", name, err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		return conn
	}

	alice := connect("Alice")
	bob := connect("Bob")
	carol := connect("Carol")
	waitForOnlineUsers := func(conn *websocket.Conn, count int) {
		t.Helper()
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		for {
			var message Message
			if err := conn.ReadJSON(&message); err != nil {
				t.Fatalf("wait for online users: %v", err)
			}
			if message.Type == "users" && len(message.Users) == count {
				return
			}
		}
	}
	waitForOnlineUsers(alice, 3)
	waitForOnlineUsers(bob, 3)
	waitForOnlineUsers(carol, 3)

	if err := alice.WriteJSON(map[string]string{"text": "private hello", "to": "Bob"}); err != nil {
		t.Fatalf("send direct message: %v", err)
	}

	readDirectMessage := func(conn *websocket.Conn, sender, recipient string) {
		t.Helper()
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		for {
			var message Message
			if err := conn.ReadJSON(&message); err != nil {
				t.Fatalf("read direct message for %s: %v", recipient, err)
			}
			if message.Type != "users" && message.Type != "direct" {
				t.Fatalf("%s received a non-direct chat message: %+v", recipient, message)
			}
			if message.Type == "direct" {
				if message.User != sender || message.To != recipient || message.Text != "private hello" {
					t.Errorf("direct message = %+v, want sender %q, recipient %q", message, sender, recipient)
				}
				return
			}
		}
	}
	readDirectMessage(alice, "Alice", "Bob")
	readDirectMessage(bob, "Alice", "Alice")

	_ = carol.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	var unrelated Message
	if err := carol.ReadJSON(&unrelated); err == nil {
		t.Errorf("unrelated client received message: %+v", unrelated)
	} else if timeout, ok := err.(interface{ Timeout() bool }); !ok || !timeout.Timeout() {
		t.Errorf("waiting for unrelated client returned %v, want a timeout", err)
	}
}

func TestMessageEncodeSupportsDirectMessageRecipient(t *testing.T) {
	raw := encode(Message{Type: "direct", User: "Alice", To: "Bob", Text: "hello"})
	var decoded map[string]string
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode encoded message: %v", err)
	}
	if decoded["type"] != "direct" || decoded["user"] != "Alice" || decoded["to"] != "Bob" {
		t.Errorf("encoded direct message has wrong fields: %s", raw)
	}
}
