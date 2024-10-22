import React from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { Provider } from "react-redux";
import { ConfigProvider as ThemeProvider } from "antd";

import DashboardPage from "./pages/Dashboard";

// client side application state
import { store } from "./store";
// client side pages

import '../public/globals.css';

const rootElement = document.getElementById("root");
const root = createRoot(rootElement);

root.render(
  <React.StrictMode>
    <Provider store={store}>
      <ThemeProvider
        theme={{
          token: {
            colorPrimary: "#ff873c",
            borderRadius: 2,
            colorBgBase: "#1e2122",
            colorBgContainer: "#1e2122",
            colorBgBase: "#353b3d",
          },
          components: {
            Layout: {
              siderBg: "#1e2122",
              headerBg: "#1e2122"
            }
          }
        }}>
        <BrowserRouter>
          <DashboardPage />
        </BrowserRouter>

      </ThemeProvider>
    </Provider>
  </React.StrictMode>
);
