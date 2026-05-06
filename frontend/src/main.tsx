import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./index.css";
import App from "./App.tsx";
import { AuthProvider } from "./lib/AuthContext";
import { BrowserRouter } from "react-router-dom";
import axios from "axios";

// Configure axios for API calls
axios.defaults.baseURL = window.location.origin;
axios.defaults.withCredentials = true;

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <AuthProvider>
        <App />
      </AuthProvider>
    </BrowserRouter>
  </StrictMode>,
);
