import * as React from 'react';
import Typography from '@mui/material/Typography';

import Title from './Title';

export default function ClusterNodes() {
  return (
    <React.Fragment>
      <Title>Cluster Details</Title>
      <Typography component="p" variant="h4">Nodes</Typography>
      <Typography color="text.secondary" sx={{ flex: 1 }}>Healthy: 25 <br/>Unhealthy: 1</Typography>
    </React.Fragment>
  );
}