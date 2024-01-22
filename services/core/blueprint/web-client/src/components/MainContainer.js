import React from "react";
import "./../index.css";

const MainContainer = ({ children, themeMode }) => (
   <div className={'main-container ${themeMode === "dark" ? "dark-mode" : ""}'}>
      {children}
   </div>
);

export default MainContainer;
