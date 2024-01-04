import * as React from 'react';
import { useSelector, useDispatch } from 'react-redux';

import Button from '@mui/material/Button';
import Stack from '@mui/material/Stack';
import Grid from '@mui/material/Grid';
import Paper from '@mui/material/Paper';

import { useGetValuesQuery } from '../services/key_value_rpc';
import { decrement, increment, incrementByAmount } from '../store/counter';

import { GetRequest, GetResponse, GetFilter } from '../grpc/registry/key_value/v1/service_pb';

export default function KeyValuesPage () {
    const count = useSelector((state) => state.counter.value)
    const dispatch = useDispatch();

    const {
        data: GetValue, 
        error: GetValueError, 
        isLoading: GetValueIsLoading
    } = useGetValuesQuery(
        {
            key: "217da09f-a8e6-472d-80bf-c15b9ce70716",
            filter: GetFilter[2],
        }
    )

    const clickApi = () => {
        console.log(GetValue)
    }

    return (
        <Grid container spacing={3}>
            <Grid item xs={12}>
                <Paper sx={{ p: 2, display: 'flex', flexDirection: 'column' }}>
                    <h2>Counter RTK Test</h2>
                    <span>{count}</span>
                    <br/>
                    <Stack spacing={2} direction="row">
                        <Button
                            variant="outlined"
                            onClick={() => dispatch(increment())}
                        >
                                Increment
                        </Button>
                        <br/>
                        <Button
                            variant="outlined"
                            onClick={() => dispatch(decrement())}
                        >
                            Decrement
                        </Button>
                        <br/>
                        <Button
                            variant="outlined"
                            onClick={() => dispatch(incrementByAmount(10))}>
                            Add 10
                        </Button>
                    </Stack>
                </Paper>
            </Grid>

            <Grid item xs={12}>
                <Paper sx={{ p: 2, display: 'flex', flexDirection: 'column' }}>
                    <h2>Set:</h2>
                    <Button 
                        variant="outlined"
                        onClick={clickApi}>Set</Button>
                </Paper>
            </Grid>

            <Grid item xs={12}>
                <Paper sx={{ p: 2, display: 'flex', flexDirection: 'column' }}>
                    <h2>Get:</h2>
                </Paper>
            </Grid>

            <Grid item xs={12}>
                <Paper sx={{ p: 2, display: 'flex', flexDirection: 'column' }}>
                    <h2>Remove:</h2>
                </Paper>
            </Grid>

            <Grid item xs={12}>
                <Paper sx={{ p: 2, display: 'flex', flexDirection: 'column' }}>
                    <h2>List:</h2>
                </Paper>
            </Grid>
        </Grid>
    )
}