import { useState } from "react";
import { useDispatch } from "react-redux";
import { signIn, signUp } from "../api/auth";
import { setSession } from "../store/chatSlice";

export default function AuthScreen() {
  const dispatch = useDispatch();
  const [mode, setMode] = useState("signin");
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const isSignUp = mode === "signup";

  function switchMode(nextMode) {
    setMode(nextMode);
    setError("");
  }

  async function handleSubmit(event) {
    event.preventDefault();
    setError("");
    if (isSignUp && password !== confirmPassword) {
      setError("Your passwords do not match.");
      return;
    }

    setSubmitting(true);
    try {
      const session = isSignUp
        ? await signUp({ username, email, password })
        : await signIn({ email, password });
      localStorage.setItem("chat-session", JSON.stringify(session));
      dispatch(setSession(session));
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="auth-page">
      <section className="auth-card" aria-labelledby="auth-title">
        <a className="brand" href="/" aria-label="Live Chat home">
          <span className="brand-mark" aria-hidden="true">✳</span>
          <span>together</span>
        </a>

        <div className="auth-heading">
          <p className="eyebrow">YOUR SPACE TO CONNECT</p>
          <h1 id="auth-title">{isSignUp ? "Create your account" : "Welcome back"}</h1>
          <p>{isSignUp ? "A little closer, one conversation at a time." : "Sign in and pick up where the conversation is."}</p>
        </div>

        <div className="auth-tabs" role="tablist" aria-label="Account access">
          <button type="button" role="tab" aria-selected={!isSignUp} className={!isSignUp ? "active" : ""} onClick={() => switchMode("signin")}>Sign in</button>
          <button type="button" role="tab" aria-selected={isSignUp} className={isSignUp ? "active" : ""} onClick={() => switchMode("signup")}>Create account</button>
        </div>

        <form className="auth-form" onSubmit={handleSubmit}>
          {isSignUp && (
            <div className="field">
              <label htmlFor="username">Your name</label>
              <input id="username" name="username" autoComplete="nickname" required minLength={2} maxLength={20} value={username} onChange={(event) => setUsername(event.target.value)} placeholder="How people will see you" />
            </div>
          )}
          <div className="field">
            <label htmlFor="email">Email address</label>
            <input id="email" name="email" type="email" autoComplete="email" required maxLength={254} value={email} onChange={(event) => setEmail(event.target.value)} placeholder="you@example.com" />
          </div>
          <div className="field">
            <label htmlFor="password">Password</label>
            <input id="password" name="password" type="password" autoComplete={isSignUp ? "new-password" : "current-password"} required minLength={8} maxLength={72} value={password} onChange={(event) => setPassword(event.target.value)} placeholder="At least 8 characters" />
          </div>
          {isSignUp && (
            <div className="field">
              <label htmlFor="confirm-password">Confirm password</label>
              <input id="confirm-password" name="confirm-password" type="password" autoComplete="new-password" required minLength={8} maxLength={72} value={confirmPassword} onChange={(event) => setConfirmPassword(event.target.value)} placeholder="Type your password again" />
            </div>
          )}

          {error && <p className="auth-error" role="alert">{error}</p>}
          <button className="auth-submit" type="submit" disabled={submitting}>
            {submitting ? "Please wait..." : isSignUp ? "Create account" : "Sign in"}
            {!submitting && <span aria-hidden="true">→</span>}
          </button>
        </form>

        <p className="auth-footer">
          {isSignUp ? "Already have an account?" : "New to together?"}{" "}
          <button type="button" onClick={() => switchMode(isSignUp ? "signin" : "signup")}>
            {isSignUp ? "Sign in" : "Create an account"}
          </button>
        </p>
      </section>
      <aside className="auth-aside" aria-label="About the chat">
        <div className="aside-orbit orbit-one" />
        <div className="aside-orbit orbit-two" />
        <div className="aside-content">
          <span className="aside-kicker"><span /> YOUR PEOPLE, RIGHT HERE</span>
          <h2>Good conversations<br />make any day <em>better.</em></h2>
          <p>A simple space for your group to catch up, share a laugh, and stay in the loop — wherever life takes you.</p>
          <div className="conversation-card">
            <div className="conversation-top"><span className="conversation-dot" /> The little things</div>
            <p>“Made it home safe. Already miss you all 🌿”</p>
            <div className="conversation-bottom"><span className="avatar avatar-a">M</span><span className="avatar avatar-b">A</span><span className="avatar avatar-c">J</span><span>your group is one message away</span></div>
          </div>
        </div>
        <span className="aside-caption">A softer place to say hello.</span>
      </aside>
    </main>
  );
}
