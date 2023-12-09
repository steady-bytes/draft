import * as React from 'react';
import Link from '@mui/material/Link';
import Typography from '@mui/material/Typography';
import Title from './Title';

function preventDefault(event) {
  event.preventDefault();
}

export default function Deposits() {
  return (
    <React.Fragment>
      <Title>Cluster Details</Title>
      <Typography component="p" variant="h4">Nodes</Typography>
      <Typography color="text.secondary" sx={{ flex: 1 }}>Healthy: 25 <br/>Unhealthy: 1</Typography>
    </React.Fragment>
  );
}