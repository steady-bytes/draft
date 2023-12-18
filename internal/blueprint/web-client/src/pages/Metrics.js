import * as React from 'react';
import Grid from '@mui/material/Grid';
import Paper from '@mui/material/Paper';
import { createConnectTransport } from "@connectrpc/connect-web";
import { createCallbackClient } from '@connectrpc/connect';
import { KeyValueService } from '../grpc/registry/key_value/v1/service_connect';

import Chart from '../components/Chart';
import ClusterNodes from '../components/ClusterNodes';
import ClusterNodesList from '../components/ClusterNodesList';

export default function MetricsPage() {
    // TODO -> make a grpc provider
    const transport = createConnectTransport({
        baseUrl: "http://localhost:2221",
    });

    const key_value_client = createCallbackClient(KeyValueService, transport);

    const handleClick = () => {
        key_value_client.set({key: "andrew", value: "needs to take a break"}, (err, res) => {
            if (!err) {
                console.log(res);
            }
        })
    }

    return (
        <Grid container spacing={3}>
            <Grid item xs={12} md={8} lg={9}>
            <Paper sx={{p: 2, display: 'flex', flexDirection: 'column', height: 240}}>
                <Chart />
            </Paper>
            </Grid>

            <Grid item xs={12} md={4} lg={3}>
                <Paper sx={{ p: 2, display: 'flex', flexDirection: 'column', height: 240 }} >
                    <ClusterNodes />
                    <button onClick={handleClick}>Click me</button>
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