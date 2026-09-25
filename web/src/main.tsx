import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import "./azrty/styles.css";
import "./app.css";
import App from "./App";
import { PRODUCT } from "./brand";
import { applyTheme, storedTheme } from "./theme";

// Apply the theme before the first paint; the CSP forbids an inline script.
applyTheme(storedTheme());
document.documentElement.dataset.pillar = "build";

const icon = document.createElement("link");
icon.rel = "icon";
icon.type = "image/png";
icon.href = PRODUCT.emblem;
document.head.appendChild(icon);

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </StrictMode>,
);
