import React from "react";
import { createContext, useContext } from "react";

const ThemeContext = createContext();

export const useTheme = () => {
   return useContext(ThemeContext);
};

export const ThemeProvider = ({ children, theme }) => {
   return (
      <ThemeContext.Provider value={theme}>{children}</ThemeContext.Provider>
   );
};
