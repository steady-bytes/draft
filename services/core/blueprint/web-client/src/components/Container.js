import React from "react";
import "./../index.css";

const Container = ({ children, themeMode }) => (
   <div className={'container ${themeMode === "dark" ? "dark-mode" : ""}'}>
      {children}
   </div>
);

export default Container;
