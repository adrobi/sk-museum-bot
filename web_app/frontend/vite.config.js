import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

const API_URL = process.env.VITE_API_URL || "http://localhost:8000";

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": API_URL,
    },
    host: "0.0.0.0",
    port: 5173,
    allowedHosts: [
      ".ngrok-free.dev" // Разрешаем доступ с любых бесплатных доменов ngrok
    ],
  },
});