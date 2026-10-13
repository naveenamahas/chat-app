package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestIssueAndVerifyToken(t *testing.T) {
	auth := &authService{signingKey: []byte("test-signing-key-with-at-least-32-bytes")}
	token, err := auth.issueToken("Ravi")
	if err != nil {
		t.Fatalf("issueToken() error = %v", err)
	}

	claims, ok := auth.verifyToken(token)
	if !ok {
		t.Fatal("verifyToken() rejected a valid token")
	}
	if claims.Username != "Ravi" {
		t.Errorf("username = %q, want Ravi", claims.Username)
	}
}

func TestVerifyTokenRejectsInvalidTokens(t *testing.T) {
	auth := &authService{signingKey: []byte("test-signing-key-with-at-least-32-bytes")}
	token, err := auth.issueToken("Ravi")
	if err != nil {
		t.Fatalf("issueToken() error = %v", err)
	}
	tampered := token[:len(token)-1] + "x"

	for _, invalid := range []string{"", "not.a.token", tampered} {
		if _, ok := auth.verifyToken(invalid); ok {
			t.Errorf("verifyToken(%q) accepted an invalid token", invalid)
		}
	}
}

func TestVerifyTokenRejectsExpiredToken(t *testing.T) {
	auth := &authService{signingKey: []byte("test-signing-key-with-at-least-32-bytes")}
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	payload, _ := json.Marshal(tokenClaims{Username: "Ravi", Expires: time.Now().Add(-time.Minute).Unix()})
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, auth.signingKey)
	_, _ = mac.Write([]byte(unsigned))
	token := unsigned + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if _, ok := auth.verifyToken(token); ok {
		t.Fatal("verifyToken() accepted an expired token")
	}
}

func TestHandleMeRequiresAndAcceptsToken(t *testing.T) {
	auth := &authService{signingKey: []byte("test-signing-key-with-at-least-32-bytes")}
	token, err := auth.issueToken("Ravi")
	if err != nil {
		t.Fatalf("issueToken() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	auth.handleMe(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("authenticated /api/me status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Body.String(); got != "{\"username\":\"Ravi\"}\n" {
		t.Errorf("authenticated /api/me body = %q", got)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/me", nil)
	response = httptest.NewRecorder()
	auth.handleMe(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated /api/me status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestAccountFlowWithMongo(t *testing.T) {
	uri := os.Getenv("AUTH_TEST_MONGODB_URI")
	if uri == "" {
		t.Skip("set AUTH_TEST_MONGODB_URI to run MongoDB auth integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	cancel()
	if err != nil {
		t.Fatalf("connect to MongoDB: %v", err)
	}
	db := client.Database("chat_auth_test_" + strconv.FormatInt(time.Now().UnixNano(), 10))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := db.Drop(ctx); err != nil {
			t.Errorf("drop test database: %v", err)
		}
		if err := client.Disconnect(ctx); err != nil {
			t.Errorf("disconnect from MongoDB: %v", err)
		}
	})

	auth := &authService{
		users:      db.Collection("users"),
		messages:   db.Collection("messages"),
		signingKey: []byte("integration-test-signing-key-with-32-bytes"),
	}
	_, err = auth.users.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "username", Value: 1}}, Options: options.Index().SetUnique(true)},
	})
	if err != nil {
		t.Fatalf("create test indexes: %v", err)
	}

	mux := http.NewServeMux()
	auth.routes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	suffix := strconv.FormatInt(time.Now().UnixNano()%1_000_000_000_000, 10)
	details := authRequest{
		Username: "Tester" + suffix,
		Email:    "tester" + suffix + "@example.com",
		Password: "strong-password",
	}
	signup := postAuthJSON(t, server.Client(), server.URL+"/api/auth/signup", details)
	if signup.StatusCode != http.StatusOK {
		t.Fatalf("signup status = %d, want %d", signup.StatusCode, http.StatusOK)
	}
	var session struct {
		Token    string `json:"token"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(signup.Body).Decode(&session); err != nil {
		signup.Body.Close()
		t.Fatalf("decode signup response: %v", err)
	}
	signup.Body.Close()
	if session.Token == "" || session.Username != details.Username {
		t.Fatalf("signup session = %+v", session)
	}

	duplicate := postAuthJSON(t, server.Client(), server.URL+"/api/auth/signup", details)
	duplicate.Body.Close()
	if duplicate.StatusCode != http.StatusConflict {
		t.Errorf("duplicate signup status = %d, want %d", duplicate.StatusCode, http.StatusConflict)
	}

	login := postAuthJSON(t, server.Client(), server.URL+"/api/auth/login", authRequest{
		Email: details.Email, Password: details.Password,
	})
	login.Body.Close()
	if login.StatusCode != http.StatusOK {
		t.Errorf("login status = %d, want %d", login.StatusCode, http.StatusOK)
	}

	wrongPassword := postAuthJSON(t, server.Client(), server.URL+"/api/auth/login", authRequest{
		Email: details.Email, Password: "incorrect-password",
	})
	wrongPassword.Body.Close()
	if wrongPassword.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong-password login status = %d, want %d", wrongPassword.StatusCode, http.StatusUnauthorized)
	}

	request, err := http.NewRequest(http.MethodGet, server.URL+"/api/me", nil)
	if err != nil {
		t.Fatalf("create /api/me request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+session.Token)
	currentUser, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("request /api/me: %v", err)
	}
	currentUser.Body.Close()
	if currentUser.StatusCode != http.StatusOK {
		t.Errorf("/api/me status = %d, want %d", currentUser.StatusCode, http.StatusOK)
	}

	hub := newHub(auth.messages)
	groupMessage, err := hub.saveMessage(Message{Type: "message", User: details.Username, Text: "saved group message"})
	if err != nil {
		t.Fatalf("save group message: %v", err)
	}
	directMessage, err := hub.saveMessage(Message{Type: "direct", User: "Friend", To: details.Username, Text: "saved direct message"})
	if err != nil {
		t.Fatalf("save direct message: %v", err)
	}

	assertHistory := func(conversation, wantType, wantUser, wantTo, wantText string) {
		t.Helper()
		request, err := http.NewRequest(http.MethodGet, server.URL+"/api/messages?conversation="+url.QueryEscape(conversation), nil)
		if err != nil {
			t.Fatalf("create history request: %v", err)
		}
		request.Header.Set("Authorization", "Bearer "+session.Token)
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatalf("request history: %v", err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("history status = %d, want %d", response.StatusCode, http.StatusOK)
		}
		var history []Message
		if err := json.NewDecoder(response.Body).Decode(&history); err != nil {
			t.Fatalf("decode history: %v", err)
		}
		if len(history) != 1 {
			t.Fatalf("history has %d messages, want one: %+v", len(history), history)
		}
		message := history[0]
		if message.Type != wantType || message.User != wantUser || message.To != wantTo || message.Text != wantText {
			t.Errorf("history message = %+v, want type %q, user %q, to %q, text %q", message, wantType, wantUser, wantTo, wantText)
		}
	}
	assertHistory("group", "message", details.Username, "", groupMessage.Text)
	assertHistory("Friend", "direct", "Friend", details.Username, directMessage.Text)

	friendToken, err := auth.issueToken("Friend")
	if err != nil {
		t.Fatalf("issue friend token: %v", err)
	}
	request, err = http.NewRequest(http.MethodGet, server.URL+"/api/messages?conversation="+url.QueryEscape(details.Username), nil)
	if err != nil {
		t.Fatalf("create friend history request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+friendToken)
	friendHistory, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("request friend history: %v", err)
	}
	defer friendHistory.Body.Close()
	if friendHistory.StatusCode != http.StatusOK {
		t.Fatalf("friend history status = %d, want %d", friendHistory.StatusCode, http.StatusOK)
	}
	var receivedHistory []Message
	if err := json.NewDecoder(friendHistory.Body).Decode(&receivedHistory); err != nil {
		t.Fatalf("decode friend history: %v", err)
	}
	if len(receivedHistory) != 1 {
		t.Fatalf("friend history has %d messages, want one: %+v", len(receivedHistory), receivedHistory)
	}
	receivedMessage := receivedHistory[0]
	if receivedMessage.User != "Friend" || receivedMessage.To != details.Username || receivedMessage.Text != directMessage.Text {
		t.Errorf("friend's sent-message history = %+v, want sender Friend and recipient %q", receivedMessage, details.Username)
	}
}

func postAuthJSON(t *testing.T, client *http.Client, url string, value any) *http.Response {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	response, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post %s: %v", url, err)
	}
	return response
}
