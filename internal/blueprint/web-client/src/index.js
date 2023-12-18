import React from 'react';
import { createRoot }from 'react-dom/client';
import { BrowserRouter } from "react-router-dom";
import { createTheme } from '@mui/material/styles';
import { createConnectTransport } from "@connectrpc/connect-web";
import { TransportProvider } from "@connectrpc/connect-query";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import Dashboard from './pages/DashboardViewport';

export const defaultTheme = createTheme({});

const rootElement = document.getElementById("root");
const root = createRoot(rootElement);

root.render(
  <React.StrictMode>
    <BrowserRouter> 
      <Dashboard />
    </BrowserRouter>
  </React.StrictMode>
);
