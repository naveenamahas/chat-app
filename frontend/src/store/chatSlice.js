import { createSlice } from "@reduxjs/toolkit";

const MAX_MESSAGES = 500; // keep memory small in long sessions

function emptyState() {
  return {
    username: "",
    token: "",
    activeConversation: "group",
    status: "idle",
    messages: [],
    users: [],
    unreadByUser: {},
    nextId: 1,
    historyLoadingFor: "",
    historyErrorFor: "",
    historyErrorMessage: "",
  };
}

let savedSession = null;
try {
  savedSession = JSON.parse(localStorage.getItem("chat-session") || "null");
} catch {
  localStorage.removeItem("chat-session");
}

function getSavedConversation(username) {
  if (!username) return "group";
  try {
    const conversation = localStorage.getItem(`chat-active-conversation:${username}`);
    return conversation && conversation !== username ? conversation : "group";
  } catch {
    return "group";
  }
}

const initialState = {
  ...emptyState(),
  username: savedSession?.username || "",
  token: savedSession?.token || "",
  activeConversation: getSavedConversation(savedSession?.username),
};

const chatSlice = createSlice({
  name: "chat",
  initialState,
  reducers: {
    setSession(state, action) {
      state.username = action.payload.username;
      state.token = action.payload.token;
    },
    leave() {
      return emptyState();
    },
    setStatus(state, action) {
      state.status = action.payload;
    },
    messageReceived(state, action) {
      if (action.payload.id && state.messages.some((message) => message.id === action.payload.id)) return;
      state.messages.push({ id: state.nextId++, ...action.payload });
      state.messages.sort((first, second) => new Date(first.time) - new Date(second.time));
      if (state.messages.length > MAX_MESSAGES) state.messages.shift();
      if (action.payload.type === "direct" && action.payload.to === state.username) {
        if (state.activeConversation === "group") {
          state.activeConversation = action.payload.user;
        } else if (state.activeConversation !== action.payload.user) {
          state.unreadByUser[action.payload.user] = (state.unreadByUser[action.payload.user] || 0) + 1;
        }
      }
    },
    setUsers(state, action) {
      state.users = action.payload;
    },
    selectConversation(state, action) {
      state.activeConversation = action.payload;
      state.historyErrorFor = "";
      state.historyErrorMessage = "";
      if (action.payload !== "group") delete state.unreadByUser[action.payload];
    },
    historyLoading(state, action) {
      state.historyLoadingFor = action.payload;
      state.historyErrorFor = "";
      state.historyErrorMessage = "";
    },
    historyReceived(state, action) {
      for (const message of action.payload.messages) {
        if (!state.messages.some((current) => current.id === message.id)) {
          state.messages.push({ id: state.nextId++, ...message });
        }
      }
      state.messages.sort((first, second) => new Date(first.time) - new Date(second.time));
      if (state.messages.length > MAX_MESSAGES) state.messages.splice(0, state.messages.length - MAX_MESSAGES);
      if (state.historyLoadingFor === action.payload.conversation) state.historyLoadingFor = "";
      if (state.historyErrorFor === action.payload.conversation) state.historyErrorFor = "";
    },
    historyFailed(state, action) {
      state.historyLoadingFor = "";
      state.historyErrorFor = action.payload.conversation;
      state.historyErrorMessage = action.payload.message;
    },
  },
});

export const {
  setSession,
  leave,
  setStatus,
  messageReceived,
  setUsers,
  selectConversation,
  historyLoading,
  historyReceived,
  historyFailed,
} = chatSlice.actions;
export default chatSlice.reducer;
