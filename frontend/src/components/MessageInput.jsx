import { useEffect, useState } from "react";
import { useSelector } from "react-redux";

// "send" is passed down as a prop from ChatRoom.
export default function MessageInput({ send }) {
  const { status, activeConversation } = useSelector((state) => state.chat);
  const isDirect = activeConversation !== "group";
  const [text, setText] = useState("");

  useEffect(() => {
    setText("");
  }, [activeConversation]);

  function handleSubmit(event) {
    event.preventDefault();
    const trimmed = text.trim();
    if (!trimmed) return;
    if (send(trimmed, isDirect ? activeConversation : "")) setText("");
  }

  return (
    <form className="composer" onSubmit={handleSubmit}>
      <input
        value={text}
        maxLength={500}
        placeholder={status !== "open" ? "Connecting..." : isDirect ? `Message ${activeConversation} privately` : "Type a message"}
        disabled={status !== "open"}
        onChange={(e) => setText(e.target.value)}
        aria-label="Message"
      />
      <button type="submit" disabled={status !== "open" || !text.trim()}>Send</button>
    </form>
  );
}
