import * as React from 'react';

import Grid from '@mui/material/Grid';
import Paper from '@mui/material/Paper';
import Typography from '@mui/material/Typography';

import Title from '../components/Title';

export default function ServiceRegistryPage () {
    return (
        <Grid container spacing={3}>
            <Grid item xs={12} md={4} lg={3}>
                <Paper sx={{ p: 2, display: 'flex', flexDirection: 'column', height: 160}} >
                    <React.Fragment>
                        <Title>Service Inventory</Title>
                        <Typography component="p" variant="h4">Healthy: 1000</Typography>
                        <Typography component="p" variant="h4">Unhealthy: 25</Typography>
                    </React.Fragment>
                </Paper>
            </Grid>

            <Grid item xs={12}>
                <Paper sx={{ p: 2, display: 'flex', flexDirection: 'column' }}>
                    <h2>Service Registry</h2>
                </Paper>
            </Grid>
        </Grid>
    )
}