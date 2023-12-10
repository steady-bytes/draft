import * as React from 'react';
import Grid from '@mui/material/Grid';
import Paper from '@mui/material/Paper';

import Chart from '../components/Chart';
import ClusterNodes from '../components/ClusterNodes';
import ClusterNodesList from '../components/ClusterNodesList';

export default function MetricsPage() {
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