import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { App } from "./App";
import { ApiProvider } from "./api/ApiProvider";
import { createDefaultClient } from "./api";
import { initTheme } from "./lib/theme";
import "./styles/tokens.css";
import "./styles/base.css";

initTheme();

const root = document.getElementById("root");
if (!root) throw new Error("Missing #root element");

createRoot(root).render(
  <StrictMode>
    <ApiProvider client={createDefaultClient()}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </ApiProvider>
  </StrictMode>,
);
