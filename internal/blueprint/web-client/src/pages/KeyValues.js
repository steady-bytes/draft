import * as React from 'react';

import Grid from '@mui/material/Grid';
import Paper from '@mui/material/Paper';

export default function KeyValuesPage () {
    return (
        <Grid container spacing={3}>
            <Grid item xs={12}>
                <Paper sx={{ p: 2, display: 'flex', flexDirection: 'column' }}>
                    <h2>Key/Values</h2>
                </Paper>
            </Grid>
        </Grid>
    )
}