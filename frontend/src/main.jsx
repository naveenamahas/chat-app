import { createRoot } from "react-dom/client";
import { Provider } from "react-redux";
import App from "./App";
import { store } from "./store";
import "./styles.css";

// StrictMode is left off on purpose: in development it opens each WebSocket twice,
// which would show duplicate "joined the chat" messages while you are learning.
createRoot(document.getElementById("root")).render(
  <Provider store={store}>
    <App />
  </Provider>
);
