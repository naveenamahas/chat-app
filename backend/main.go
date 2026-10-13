package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// allowedOrigins comes from the ALLOWED_ORIGINS env variable (comma separated),
// for example: https://my-chat.onrender.com
// localhost is always allowed so local development works without setup.
func originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // non-browser clients
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" {
		return true
	}
	for _, allowed := range strings.Split(os.Getenv("ALLOWED_ORIGINS"), ",") {
		if strings.TrimSpace(allowed) == origin {
			return true
		}
	}
	return false
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     originAllowed,
}

// serveWS upgrades a normal HTTP request to a WebSocket connection.
// The first WebSocket message authenticates the connection without placing the token in the URL.
func serveWS(hub *Hub, auth *authService, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("upgrade error:", err)
		return
	}

	conn.SetReadLimit(4096)
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, raw, err := conn.ReadMessage()
	var handshake struct {
		Type  string `json:"type"`
		Token string `json:"token"`
	}
	if err != nil || json.Unmarshal(raw, &handshake) != nil || handshake.Type != "auth" {
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "authentication required"), time.Now().Add(time.Second))
		_ = conn.Close()
		return
	}
	claims, ok := auth.verifyToken(handshake.Token)
	if !ok {
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "invalid or expired token"), time.Now().Add(time.Second))
		_ = conn.Close()
		return
	}
	conn.SetReadDeadline(time.Time{})
	client := &Client{hub: hub, conn: conn, send: make(chan []byte, 256), name: cleanName(claims.Username)}
	hub.register <- client

	// One goroutine reads from the browser, another writes to the browser.
	go client.writePump()
	go client.readPump()
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && originAllowed(r) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			if origin != "" && !originAllowed(r) {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	cancel()
	if err != nil {
		log.Fatal("connect to MongoDB:", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := mongoClient.Disconnect(ctx); err != nil {
			log.Println("disconnect from MongoDB:", err)
		}
	}()
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	err = mongoClient.Ping(ctx, nil)
	cancel()
	if err != nil {
		log.Fatal("ping MongoDB:", err)
	}
	databaseName := os.Getenv("MONGODB_DATABASE")
	if databaseName == "" {
		databaseName = "chatapp"
	}
	auth, err := newAuthService(mongoClient.Database(databaseName))
	if err != nil {
		log.Fatal("configure authentication:", err)
	}

	hub := newHub(auth.messages)
	go hub.run()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWS(hub, auth, w, r)
	})
	auth.routes(mux)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	port := os.Getenv("PORT") // Render sets PORT automatically
	if port == "" {
		port = "8080"
	}
	log.Println("Chat server running on :" + port)
	log.Fatal(http.ListenAndServe(":"+port, cors(mux)))
}
