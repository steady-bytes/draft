import React from 'react';
import { createRoot }from 'react-dom/client';
import { BrowserRouter } from "react-router-dom";
import { Provider } from 'react-redux';

import { store } from './store';
import { theme } from './theme';

import Dashboard from './pages/DashboardViewport';

const rootElement = document.getElementById("root");
const root = createRoot(rootElement);

root.render(
  <React.StrictMode>
    <Provider store={store} >
      <BrowserRouter> 
        <Dashboard />
      </BrowserRouter>
    </Provider>
  </React.StrictMode>
);
