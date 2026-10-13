import { API_URL } from "../config";

async function request(path, options = {}) {
  let response;
  try {
    response = await fetch(`${API_URL}${path}`, {
      ...options,
      headers: {
        "Content-Type": "application/json",
        ...(options.token ? { Authorization: `Bearer ${options.token}` } : {}),
        ...options.headers,
      },
    });
  } catch (error) {
    if (error instanceof TypeError) {
      throw new Error(`Can't reach the chat server at ${API_URL}. Check that the backend is running.`);
    }
    throw error;
  }
  if (!response.ok) {
    const message = (await response.text()).trim();
    const error = new Error(message || "Something went wrong. Please try again.");
    error.status = response.status;
    throw error;
  }
  return response.json();
}

export const signUp = (details) =>
  request("/api/auth/signup", { method: "POST", body: JSON.stringify(details) });

export const signIn = (details) =>
  request("/api/auth/login", { method: "POST", body: JSON.stringify(details) });

export const getCurrentUser = (token) => request("/api/me", { token });
