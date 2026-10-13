import { API_URL } from "../config";

export async function fetchMessageHistory(token, conversation) {
  const query = new URLSearchParams({ conversation });
  let response;
  try {
    response = await fetch(`${API_URL}/api/messages?${query}`, {
      headers: { Authorization: `Bearer ${token}` },
    });
  } catch (error) {
    if (error instanceof TypeError) {
      throw new Error(`Can't reach the chat server at ${API_URL}. Check that the backend is running.`);
    }
    throw error;
  }
  if (!response.ok) {
    const message = (await response.text()).trim();
    throw new Error(message || "Could not load message history.");
  }
  return response.json();
}
