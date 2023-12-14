import * as React from 'react';
import * as ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { createTheme } from '@mui/material/styles';

import Dashboard from './pages/DashboardViewport';

export const defaultTheme = createTheme({
    // palette: {
    //     mode: 'dark',
    // },
});

// const router = createBrowserRouter([
//   {
//     path: "/",
//     element: <MetricsDashboard />,
//     errorElement: <ErrorPage />,
//   }
// ]);

ReactDOM.createRoot(document.getElementById("root")).render(
  <React.StrictMode>
    <BrowserRouter>
      <Dashboard />
    </BrowserRouter>
  </React.StrictMode>
);
