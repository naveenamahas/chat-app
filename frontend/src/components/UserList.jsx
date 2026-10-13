import { useDispatch, useSelector } from "react-redux";
import { selectConversation } from "../store/chatSlice";

export default function UserList() {
  const dispatch = useDispatch();
  const { users, username, activeConversation, unreadByUser = {} } = useSelector((state) => state.chat);
  return (
    <aside className="users">
      <h2>Conversations</h2>
      <ul>
        <li>
          <button
            type="button"
            className={`conversation-choice ${activeConversation === "group" ? "selected" : ""}`}
            aria-pressed={activeConversation === "group"}
            onClick={() => dispatch(selectConversation("group"))}
          >
            <span className="conversation-icon" aria-hidden="true">G</span>
            <span className="conversation-name">Group chat</span>
          </button>
        </li>
        <li className="users-label">ONLINE · {users.length}</li>
        {users.map((name) => (
          <li key={name}>
            {name === username ? (
              <div className="conversation-choice self">
                <span className="dot" />
                <span className="conversation-name">{name}</span>
                <em>You</em>
              </div>
            ) : (
              <button
                type="button"
                className={`conversation-choice ${activeConversation === name ? "selected" : ""}`}
                aria-pressed={activeConversation === name}
                onClick={() => dispatch(selectConversation(name))}
              >
                <span className="dot" />
                <span className="conversation-name">{name}</span>
                {unreadByUser[name] > 0 && (
                  <span className="unread-count" aria-label={`${unreadByUser[name]} unread messages`}>
                    {unreadByUser[name]}
                  </span>
                )}
              </button>
            )}
          </li>
        ))}
      </ul>
    </aside>
  );
}
