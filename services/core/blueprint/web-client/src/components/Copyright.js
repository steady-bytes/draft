import * as React from "react";
import "./../index.css";

export default function Copyright(props) {
   return (
      <div className="footer">
         <p>&copy; {new Date().getFullYear()} steady-bytes</p>
      </div>
   );
}
