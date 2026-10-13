import { useEffect, useRef } from "react";
import { useSelector } from "react-redux";

const formatTime = (iso) =>
  new Date(iso).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });

export default function MessageList() {
  const { messages, username, activeConversation, historyLoadingFor, historyErrorFor } = useSelector((state) => state.chat);
  const endRef = useRef(null);
  const conversationMessages = messages.filter((message) => {
    if (message.type === "system") {
      return activeConversation === "group" || message.to === activeConversation;
    }
    if (message.type === "direct") {
      return activeConversation !== "group" &&
        ((message.user === username && message.to === activeConversation) ||
          (message.user === activeConversation && message.to === username));
    }
    return activeConversation === "group" && message.type === "message";
  });

  // Scroll to the newest message whenever the list changes.
  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [conversationMessages.length, activeConversation]);

  return (
    <div className="messages" aria-live="polite">
      {conversationMessages.length === 0 && historyLoadingFor === activeConversation && (
        <p className="empty">Loading saved messages...</p>
      )}
      {conversationMessages.length === 0 && historyErrorFor !== activeConversation && historyLoadingFor !== activeConversation && (
        <p className="empty">
          {activeConversation === "group"
            ? "No messages yet. Say hello!"
            : `This is the start of your private conversation with ${activeConversation}.`}
        </p>
      )}
      {conversationMessages.map((m) =>
        m.type === "system" ? (
          <div key={m.id} className="system">{m.text}</div>
        ) : (
          <div key={m.id} className={`bubble-row ${m.user === username ? "mine" : "theirs"}`}>
            <div className="bubble">
              {m.user !== username && <strong className="sender">{m.user}</strong>}
              <span className="text">{m.text}</span>
              <time>{formatTime(m.time)}</time>
            </div>
          </div>
        )
      )}
      <div ref={endRef} />
    </div>
  );
}
