package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

const tokenLifetime = 24 * time.Hour

type account struct {
	Username     string `bson:"username" json:"username"`
	Email        string `bson:"email" json:"email"`
	PasswordHash string `bson:"passwordHash"`
}

type authService struct {
	users      *mongo.Collection
	messages   *mongo.Collection
	signingKey []byte
}

const messageHistoryLimit int64 = 50

type authRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type tokenClaims struct {
	Username string `json:"username"`
	Expires  int64  `json:"exp"`
}

func newAuthService(db *mongo.Database) (*authService, error) {
	key := []byte(os.Getenv("AUTH_SECRET"))
	if len(key) < 32 {
		return nil, errors.New("AUTH_SECRET must be set to at least 32 characters")
	}

	users := db.Collection("users")
	messages := db.Collection("messages")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := users.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "username", Value: 1}}, Options: options.Index().SetUnique(true)},
	})
	if err != nil {
		return nil, fmt.Errorf("create account indexes: %w", err)
	}
	_, err = messages.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "type", Value: 1}, {Key: "time", Value: -1}}},
		{Keys: bson.D{{Key: "type", Value: 1}, {Key: "user", Value: 1}, {Key: "to", Value: 1}, {Key: "time", Value: -1}}},
	})
	if err != nil {
		return nil, fmt.Errorf("create message indexes: %w", err)
	}

	return &authService{users: users, messages: messages, signingKey: key}, nil
}

func (a *authService) routes(mux *http.ServeMux) {
	mux.HandleFunc("/api/auth/signup", a.handleSignup)
	mux.HandleFunc("/api/auth/login", a.handleLogin)
	mux.HandleFunc("/api/me", a.handleMe)
	mux.HandleFunc("/api/messages", a.handleMessageHistory)
}

func (a *authService) handleMessageHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims, ok := a.claimsFromRequest(r)
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	conversation := r.URL.Query().Get("conversation")
	var filter bson.M
	switch {
	case conversation == "group":
		filter = bson.M{"type": "message"}
	case conversation != "" && conversation != claims.Username:
		filter = bson.M{
			"type": "direct",
			"$or": bson.A{
				bson.M{"user": claims.Username, "to": conversation},
				bson.M{"user": conversation, "to": claims.Username},
			},
		}
	default:
		http.Error(w, "valid conversation is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	cursor, err := a.messages.Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "time", Value: -1}}).
		SetLimit(messageHistoryLimit))
	if err != nil {
		http.Error(w, "Could not load message history", http.StatusInternalServerError)
		return
	}
	defer cursor.Close(ctx)

	stored := make([]storedMessage, 0, messageHistoryLimit)
	if err := cursor.All(ctx, &stored); err != nil {
		http.Error(w, "Could not load message history", http.StatusInternalServerError)
		return
	}
	result := make([]Message, 0, len(stored))
	for i := len(stored) - 1; i >= 0; i-- {
		message := stored[i]
		recipient := message.To
		if message.Type == "direct" && message.User != claims.Username {
			recipient = claims.Username
		}
		result = append(result, Message{
			ID: message.ID.Hex(), Type: message.Type, User: message.User,
			To: recipient, Text: message.Text, Time: message.Time.UTC().Format(time.RFC3339),
		})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
}

func (a *authService) handleSignup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var input authRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Username = strings.TrimSpace(input.Username)
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	if utf8.RuneCountInString(input.Username) < 2 || utf8.RuneCountInString(input.Username) > maxNameLength {
		http.Error(w, "Name must be between 2 and 20 characters", http.StatusBadRequest)
		return
	}
	if len(input.Email) > 254 {
		http.Error(w, "Enter a valid email address", http.StatusBadRequest)
		return
	}
	parsedEmail, err := mail.ParseAddress(input.Email)
	if err != nil || parsedEmail.Address != input.Email {
		http.Error(w, "Enter a valid email address", http.StatusBadRequest)
		return
	}
	if len(input.Password) < 8 || len(input.Password) > 72 {
		http.Error(w, "Password must be between 8 and 72 bytes", http.StatusBadRequest)
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Could not create account", http.StatusInternalServerError)
		return
	}
	user := account{Username: input.Username, Email: input.Email, PasswordHash: string(passwordHash)}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if _, err := a.users.InsertOne(ctx, user); err != nil {
		var writeErr mongo.WriteException
		if errors.As(err, &writeErr) && len(writeErr.WriteErrors) > 0 && writeErr.WriteErrors[0].Code == 11000 {
			http.Error(w, "An account with that email or name already exists", http.StatusConflict)
			return
		}
		http.Error(w, "Could not create account", http.StatusInternalServerError)
		return
	}
	a.writeAuthResponse(w, user.Username)
}

func (a *authService) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var input authRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var user account
	if err := a.users.FindOne(ctx, bson.M{"email": input.Email}).Decode(&user); err != nil ||
		bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)) != nil {
		http.Error(w, "Email or password is incorrect", http.StatusUnauthorized)
		return
	}
	a.writeAuthResponse(w, user.Username)
}

func (a *authService) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims, ok := a.claimsFromRequest(r)
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"username": claims.Username})
}

func (a *authService) writeAuthResponse(w http.ResponseWriter, username string) {
	token, err := a.issueToken(username)
	if err != nil {
		http.Error(w, "Could not create session", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]string{"token": token, "username": username})
}

func (a *authService) issueToken(username string) (string, error) {
	header, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(tokenClaims{Username: username, Expires: time.Now().Add(tokenLifetime).Unix()})
	if err != nil {
		return "", err
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, a.signingKey)
	_, _ = mac.Write([]byte(unsigned))
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (a *authService) verifyToken(token string) (tokenClaims, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return tokenClaims{}, false
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || string(header) != `{"alg":"HS256","typ":"JWT"}` {
		return tokenClaims{}, false
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return tokenClaims{}, false
	}
	mac := hmac.New(sha256.New, a.signingKey)
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return tokenClaims{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return tokenClaims{}, false
	}
	var claims tokenClaims
	if json.Unmarshal(payload, &claims) != nil || claims.Username == "" || time.Now().Unix() >= claims.Expires {
		return tokenClaims{}, false
	}
	return claims, true
}

func (a *authService) claimsFromRequest(r *http.Request) (tokenClaims, bool) {
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, prefix) {
		return tokenClaims{}, false
	}
	return a.verifyToken(strings.TrimPrefix(header, prefix))
}

func decodeJSON(w http.ResponseWriter, r *http.Request, value any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, "Could not write response", http.StatusInternalServerError)
	}
}
