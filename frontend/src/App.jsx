import { useEffect, useState } from "react";
import { useDispatch, useSelector } from "react-redux";
import { getCurrentUser } from "./api/auth";
import AuthScreen from "./components/AuthScreen";
import ChatRoom from "./components/ChatRoom";
import { leave, setSession } from "./store/chatSlice";

export default function App() {
  const dispatch = useDispatch();
  const { username, token } = useSelector((state) => state.chat);
  const [checkingSession, setCheckingSession] = useState(Boolean(token));

  useEffect(() => {
    if (!token) {
      setCheckingSession(false);
      return;
    }
    let active = true;
    getCurrentUser(token)
      .then((user) => {
        if (active) dispatch(setSession({ ...user, token }));
      })
      .catch((error) => {
        if (error.status === 401) {
          localStorage.removeItem("chat-session");
          if (active) dispatch(leave());
        }
      })
      .finally(() => {
        if (active) setCheckingSession(false);
      });
    return () => {
      active = false;
    };
  }, [dispatch, token]);

  if (checkingSession) return <main className="auth-loading">Restoring your session...</main>;
  return username && token ? <ChatRoom /> : <AuthScreen />;
}
