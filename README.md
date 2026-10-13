# Real-time Group Chat (Go + WebSocket, MongoDB, React + Redux Toolkit)

Create an account or sign in, then join the group chat. Messages appear instantly for everyone, without refreshing.

```
chat-app/
├── backend/   Go server
│   ├── main.go     starts the server, /ws (WebSocket), /health, and auth API
│   ├── auth.go     MongoDB accounts, password hashing, and signed sessions
│   ├── hub.go      Hub: keeps all clients, broadcasts every message to everyone
│   └── client.go   Client: one goroutine reads from the browser, one writes to it
└── frontend/  React + Redux Toolkit
    └── src/
        ├── api/auth.js             auth API client
        ├── components/AuthScreen.jsx sign-in and sign-up
        ├── hooks/useChatSocket.js   opens the WebSocket, auto-reconnects
        ├── store/chatSlice.js       signed-in user, selected conversation, messages, online users
        └── components/              ChatRoom, MessageList, MessageInput, UserList
```

## Run it (2 terminals)

1. Install Go (1.21 or newer) and Node.js (18 or newer).
2. Start MongoDB (local or hosted). Configure the backend environment:
   ```text
   MONGODB_URI=mongodb://localhost:27017
   MONGODB_DATABASE=chatapp
   AUTH_SECRET=<a random secret of at least 32 characters>
   ```
   Use a private, randomly generated `AUTH_SECRET` in production and keep it unchanged between deploys.
3. Terminal 1, backend:
   ```bash
   cd backend
   go run .            # http://localhost:8080
   ```
4. Terminal 2, frontend:
   ```bash
   cd frontend
   npm install
   npm run dev         # http://localhost:5173
   ```
5. Open http://localhost:5173 and create an account. Open a second browser window and sign in with a different account to test group chat.

## How it works

1. The browser opens `ws://localhost:8080/ws`. Its first WebSocket message presents the signed account token; the server verifies it before joining the room.
2. Each connection becomes a `Client` with two goroutines: `readPump` (browser to server) and `writePump` (server to browser).
3. A message with no `to` field is sent to the group; a message with a `to` username is routed only to that person's connected sessions and the sender's sessions.
4. Group and direct messages are stored in MongoDB. `GET /api/messages?conversation=group` or `GET /api/messages?conversation=<username>` returns the latest 50 messages for the authenticated user.
5. In React, `useChatSocket` receives live messages and conversation selection fetches saved history into Redux.

The server sets the sender name itself, so nobody can pretend to be another user.

## Authentication

- `POST /api/auth/signup` creates a MongoDB account and returns a signed token.
- `POST /api/auth/login` verifies a bcrypt password and returns a signed token.
- `GET /api/me` verifies the bearer token and restores the current session.
- Unique MongoDB indexes protect email addresses and usernames from duplicates.
- Set `ALLOWED_ORIGINS` to a comma-separated list of frontend origins when deployed.

The frontend uses `VITE_API_URL` for HTTP auth requests and `VITE_WS_URL` for the chat connection. Set both to the deployed backend (use HTTPS/WSS in production).

## Done so far

- Account sign-up and sign-in with MongoDB-backed accounts
- Group chat with instant delivery
- Select an online user for a private one-to-one conversation
- Show the signed-in user's name in the chat header and online list
- Save group and direct messages in MongoDB and restore the latest 50 messages per conversation after sign-in or refresh
- Join, leave and online-users updates
- Auto-reconnect and a "Connection lost" banner (needed for a sleeping free Render server)
- Message limits (500 characters), name limit (20), origin check, ping/pong to detect dead connections
- Direct messages are delivered live only while both people are online; saved messages remain available when they return
- Only new messages sent after MongoDB message history is enabled are available to restore; older messages were never stored

## What to do next (in this order)

1. **Read and understand** `hub.go` and `client.go` first. Draw the flow on paper: browser, readPump, Hub, writePump, browser.
2. **Experiment:** log every message in `hub.go`, change the message limit, add a "typing..." message type.
3. **Older message pagination:** load earlier history beyond the latest 50 messages as the user scrolls.
4. **Rooms:** add a `room` field and keep one Hub per room.
5. **Redis:** use Pub/Sub so several server instances share messages, and store online status.
6. **Deploy on Render** (see below) and test from two devices.

## Deploy on Render (free)

**Backend** (New > Web Service)
- Root Directory: `backend`, Runtime: Go
- Build Command: `go build -o app .`
- Start Command: `./app`
- Environment variables: `MONGODB_URI`, `MONGODB_DATABASE=chatapp`, a random `AUTH_SECRET` of at least 32 characters, and `ALLOWED_ORIGINS=https://YOUR-FRONTEND.onrender.com` (no trailing slash)

**Frontend** (New > Static Site)
- Root Directory: `frontend`
- Build Command: `npm ci && npm run build`
- Publish Directory: `dist`
- Environment variables: `VITE_WS_URL=wss://YOUR-BACKEND.onrender.com/ws` and `VITE_API_URL=https://YOUR-BACKEND.onrender.com`

Free Render web services sleep after about 15 minutes without requests. The first connection can take up to a minute, and the chat reconnects by itself. Open the backend URL once before a demo.
