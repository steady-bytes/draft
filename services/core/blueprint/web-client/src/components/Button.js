import React from "react";
import "./../index.css";

const Button = ({ text, onClick }) => {
  return (
    <button className="btn" onClick={onClick}>
      {text}
    </button>
  );
};

export default Button;
