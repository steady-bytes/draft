import * as React from "react";
import { Routes, Route } from "react-router-dom";
// application pages
import MetricsPage from "./Metrics";
import ServiceRegistryPage from "./ServiceRegistry";
import KeyValuesPage from "./KeyValues";
import ErrorPage from "./ErrorPage";

// application components
import Copyright from "../components/Copyright";
import { defaultTheme } from "../theme";
import MainAppBar from "../components/MainAppBar";
import MainContainer from "../components/MainContainer";
import Container from "../components/Container";
import { ThemeProvider } from "../context/ThemeContext";

export default function Dashboard() {
   return (
      <ThemeProvider theme={defaultTheme}>
         <MainAppBar />

         <MainContainer>
            <Container>
               {/* TODO -> Add router */}
               <Routes>
                  <Route index element={<MetricsPage />} />
                  <Route
                     path="service-registry"
                     element={<ServiceRegistryPage />}
                  />
                  <Route path="key-values" element={<KeyValuesPage />} />
                  <Route path="*" element={<ErrorPage />} />
               </Routes>
               {/* TODO -> wrap in a sticky footer component */}
            </Container>
            <Copyright />
         </MainContainer>
      </ThemeProvider>
   );
}
