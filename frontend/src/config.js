// WebSocket address of the Go server. Set VITE_WS_URL in .env.local / .env.production.
export const WS_URL = import.meta.env.VITE_WS_URL || "ws://localhost:8080/ws";
// HTTP address of the Go server. Set VITE_API_URL when the frontend and backend differ.
export const API_URL = import.meta.env.VITE_API_URL || "http://localhost:8080";
