import React from 'react';
import { useQuery } from '@connectrpc/connect-query';

import { query } from 'api/core/registry/service_discovery/v1/service-ServiceDiscoveryService_connectquery'
import { Filter, QueryRequest } from 'api/core/registry/service_discovery/v1/service_pb'

const ServiceRegistryPage: React.FC = () => {
    const {data, error, isLoading } = useQuery(query, new QueryRequest({
        filter: new Filter({})
    }),{
        select: (data) => {
            console.log(data)
            for(const key in data) {
                if (Object.hasOwnProperty.call(data, key)) {
                    console.log(key, data[key]);
                }
            }
        }
    })
    return (
        <div>
            <h2>Registry</h2>
            <div></div>
        </div>
    )
}

export default ServiceRegistryPage;