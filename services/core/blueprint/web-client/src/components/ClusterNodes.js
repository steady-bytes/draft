import * as React from "react";
import "./../index.css";

import Title from "./Title";

export default function ClusterNodes() {
  return (
    <>
      <Title text="Cluster Details" />
      <h4>Nodes</h4>
      <div className="clusterNodes-content">
        Healthy: 25 <br />
        Unhealthy: 1
      </div>
    </>
  );
}
