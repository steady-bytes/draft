import * as React from 'react';
import { Routes, Route } from "react-router-dom";

import Box from '@mui/material/Box';
import Container from '@mui/material/Container';
import CssBaseline from '@mui/material/CssBaseline';
import Toolbar from '@mui/material/Toolbar';
import { ThemeProvider } from '@mui/material/styles';
// application pages
import MetricsPage from './Metrics';
import ServiceRegistryPage from './ServiceRegistry';
import KeyValuesPage from './KeyValues';
import ErrorPage from './ErrorPage';

// application components
import Copyright from '../components/Copyright';
import { defaultTheme } from '../theme';
import MainAppBar from '../components/MainAppBar';

export default function Dashboard() {
  return (
    <ThemeProvider theme={defaultTheme}>
      <Box sx={{ display: 'flex' }}>
        <CssBaseline />
        <MainAppBar /> 

        <Box
          component="main"
          sx={{
            backgroundColor: (theme) =>
              theme.palette.mode === 'light'
                ? theme.palette.grey[100]
                : theme.palette.grey[900],
            flexGrow: 1,
            height: '100vh',
            overflow: 'auto',
          }} >

          <Toolbar />

          <Container maxWidth="lg" sx={{ mt: 4, mb: 4 }}>
            {/* TODO -> Add router */}
            <Routes>
              <Route index element={<MetricsPage />} />
              <Route path="service-registry" element={<ServiceRegistryPage />} />
              <Route path="key-values" element={<KeyValuesPage />} />
              <Route path="*" element={<ErrorPage />} />
            </Routes>
            {/* TODO -> wrap in a sticky footer component */}
            <Copyright sx={{ pt: 4 }} />
          </Container>
        </Box>

      </Box>
    </ThemeProvider>
  );
}