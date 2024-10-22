import { createApi, fetchBaseQuery } from '@reduxjs/toolkit/query/react';
import { createConnectTransport } from "@connectrpc/connect-web";
import { createPromiseClient } from '@connectrpc/connect';
import * as Config from '../utils/config';

import { KeyValueService } from 'api/core/registry/key_value/v1/service_connect';
// import { GetRequest, GetResponse, GetFilter } from '../grpc/registry/key_value/v1/service_pb';

const key_value_transport = createConnectTransport({
    baseUrl: Config.BASE_URL,
});

const client = createPromiseClient(KeyValueService, key_value_transport);

export const keyValueRPCService = createApi({
    reducerPath: 'key_value_service',
    baseQuery: fetchBaseQuery({ baseUrl: Config.BASE_URL }),
    endpoints: (builder) => ({
        getValues: builder.query({
            queryFn: async(req) => { 
                const res = await client.get(req)
                return { data: { value: JSON.parse(res.response.value) }}
            }
        }),
        setValue: builder.mutation({
            queryFn: async () => {
                try {
                    // TODO -> determin a better way to build the request
                    //         I'm pretty certain connect has a way of building these
                    const req = {};
                    const res = await client.set(req)

                    return { data: res.toJson() };
                } catch (error) {
                    return { error: error.rawMessage };
                }
            }
        })
    }),
})

export const { useGetValuesQuery } = keyValueRPCService;