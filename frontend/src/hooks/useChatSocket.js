import { useCallback, useEffect, useRef } from "react";
import { useDispatch } from "react-redux";
import { WS_URL } from "../config";
import { leave, messageReceived, setStatus, setUsers } from "../store/chatSlice";

// Opens the WebSocket, pushes everything the server sends into Redux,
// and reconnects automatically if the connection drops
// (this matters on a free Render server, which sleeps when idle).
export function useChatSocket(username, token) {
  const dispatch = useDispatch();
  const socketRef = useRef(null);

  useEffect(() => {
    let stopped = false; // true after the user leaves the chat
    let retries = 0;
    let timer;

    function connect() {
      dispatch(setStatus(retries === 0 ? "connecting" : "reconnecting"));
      const socket = new WebSocket(WS_URL);
      socketRef.current = socket;

      socket.onopen = () => {
        socket.send(JSON.stringify({ type: "auth", token }));
        retries = 0;
        dispatch(setStatus("open"));
      };

      socket.onmessage = (event) => {
        const data = JSON.parse(event.data);
        if (data.type === "users") dispatch(setUsers(data.users));
        else dispatch(messageReceived(data));
      };

      socket.onclose = (event) => {
        if (stopped) return;
        if (event.code === 1008) {
          localStorage.removeItem("chat-session");
          dispatch(leave());
          return;
        }
        dispatch(setStatus("reconnecting"));
        retries += 1;
        timer = setTimeout(connect, Math.min(1000 * 2 ** retries, 10000)); // 2s, 4s, 8s, 10s...
      };
    }

    connect();

    return () => {
      stopped = true;
      clearTimeout(timer);
      socketRef.current?.close();
    };
  }, [username, token, dispatch]);

  // Returns true if the message was sent, false if the connection is not open.
  const send = useCallback((text, recipient = "") => {
    const socket = socketRef.current;
    if (!socket || socket.readyState !== WebSocket.OPEN) return false;
    socket.send(JSON.stringify({ text, ...(recipient ? { to: recipient } : {}) }));
    return true;
  }, []);

  return send;
}
