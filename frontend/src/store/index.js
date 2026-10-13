import { configureStore } from "@reduxjs/toolkit";
import chat from "./chatSlice";

export const store = configureStore({ reducer: { chat } });
