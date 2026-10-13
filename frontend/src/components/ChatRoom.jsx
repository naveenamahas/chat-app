import { useEffect, useState } from "react";
import { useDispatch, useSelector } from "react-redux";
import { fetchMessageHistory } from "../api/messages";
import { useChatSocket } from "../hooks/useChatSocket";
import { historyFailed, historyLoading, historyReceived, leave } from "../store/chatSlice";
import MessageInput from "./MessageInput";
import MessageList from "./MessageList";
import UserList from "./UserList";

const STATUS_TEXT = {
  connecting: "Connecting...",
  open: "Connected",
  reconnecting: "Reconnecting...",
};

const HISTORY_REFRESH_INTERVAL = 3000;

export default function ChatRoom() {
  const dispatch = useDispatch();
  const { username, token, status, users, activeConversation, historyErrorFor, historyErrorMessage } = useSelector((state) => state.chat);
  const [historyRetry, setHistoryRetry] = useState(0);
  const send = useChatSocket(username, token);

  useEffect(() => {
    localStorage.setItem(`chat-active-conversation:${username}`, activeConversation);
  }, [activeConversation, username]);

  useEffect(() => {
    let active = true;
    let loading = false;
    dispatch(historyLoading(activeConversation));
    async function loadHistory() {
      if (loading) return;
      loading = true;
      try {
        const messages = await fetchMessageHistory(token, activeConversation);
        if (active) dispatch(historyReceived({ conversation: activeConversation, messages }));
      } catch (error) {
        if (active) dispatch(historyFailed({ conversation: activeConversation, message: error.message }));
      } finally {
        loading = false;
      }
    }

    loadHistory();
    const interval = window.setInterval(loadHistory, HISTORY_REFRESH_INTERVAL);
    return () => {
      active = false;
      window.clearInterval(interval);
    };
  }, [activeConversation, dispatch, historyRetry, token]);

  function handleSignOut() {
    localStorage.removeItem("chat-session");
    localStorage.removeItem(`chat-active-conversation:${username}`);
    dispatch(leave());
  }

  return (
    <div className="room">
      <header className="room-header">
        <div>
          <h1>{activeConversation === "group" ? "Group chat" : activeConversation}</h1>
          <span className={`status ${status}`}>
            <span className="dot" /> {STATUS_TEXT[status] || ""} · {users.length} online
          </span>
          <span className="conversation-privacy">
            {activeConversation === "group"
              ? "Public · visible to everyone online"
              : `Private · only you and ${activeConversation}`}
          </span>
        </div>
        <div className="header-actions">
          <span className="account-name">
            <span className="account-avatar">{username.slice(0, 1).toUpperCase()}</span>
            {username}
          </span>
          <button className="ghost" type="button" onClick={handleSignOut}>Sign out</button>
        </div>
      </header>

      {status === "reconnecting" && (
        <div className="banner">Connection lost. Trying to reconnect (a sleeping server can take up to a minute)...</div>
      )}

      <div className="room-body">
        <section className="chat">
          {historyErrorFor === activeConversation && (
            <div className="history-error" role="alert">
              {historyErrorMessage}
              <button type="button" onClick={() => setHistoryRetry((attempt) => attempt + 1)}>Try again</button>
            </div>
          )}
          <MessageList />
          <MessageInput send={send} />
        </section>
        <UserList />
      </div>
    </div>
  );
}
