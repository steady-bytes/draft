import * as React from "react";
import PropTypes from "prop-types";

function Title(props) {
  return <h2 className="title">{props.children}</h2>;
}

export default Title;
