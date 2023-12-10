import React from 'react';
import { createRoot }from 'react-dom/client';
import { createBrowserRouter, RouterProvider } from "react-router-dom";
import { createTheme } from '@mui/material/styles';

import Dashboard from './pages/DashboardViewport';

export const defaultTheme = createTheme({
    palette: {
        mode: 'dark',
    },
});

const router = createBrowserRouter([
  {
    path: "/",
    element: <Dashboard />,
  },
]);

const rootElement = document.getElementById("root");
const root = createRoot(rootElement);

root.render(
    <div>
      <RouterProvider router={router} />
    </div>
  );
