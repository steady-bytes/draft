import * as React from 'react';
import Grid from '@mui/material/Grid';
import Paper from '@mui/material/Paper';

import Chart from '../components/Chart';
import ClusterNodes from '../components/ClusterNodes';
import ClusterNodesList from '../components/ClusterNodesList';

import { createCallbackClient } from '@connectrpc/connect';
import { createConnectTransport } from "@connectrpc/connect-web";
import { KeyValueService } from '../grpc/registry/key_value/v1/service_connect';

// This transport is going to be used throughout the app
const transport = createConnectTransport({
    baseUrl: "http://localhost:2221",
  });

export default function MetricsPage() {
    const client = createCallbackClient(KeyValueService, transport);
    
    const handleClick = () => {
        client.set({key: "andrew", value: "needs to take a break"}, (err, res) => {
            if (!err) {
                console.log(res);
              }
        })
    }

    return (
        <Grid container spacing={3}>
            <button onClick={handleClick}>Click Me</button>
            <Grid item xs={12} md={8} lg={9}>
            <Paper sx={{p: 2, display: 'flex', flexDirection: 'column', height: 240}}>
                <Chart />
            </Paper>
            </Grid>

            <Grid item xs={12} md={4} lg={3}>
                <Paper sx={{ p: 2, display: 'flex', flexDirection: 'column', height: 240 }} >
                    <ClusterNodes />
                </Paper>
            </Grid>

            <Grid item xs={12}>
                <Paper sx={{ p: 2, display: 'flex', flexDirection: 'column' }}>
                    <ClusterNodesList />
                </Paper>
            </Grid>
        </Grid> 
    )
}