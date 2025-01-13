import React from 'react';
import { useQuery } from '@connectrpc/connect-query';
import type { ConnectError } from '@connectrpc/connect';

import { Table, Flex, Spin, notification, NotificationArgsProps } from "antd";
import { LoadingOutlined } from "@ant-design/icons"

import { query } from 'api/core/registry/service_discovery/v1/service-ServiceDiscoveryService_connectquery'
import { Filter, QueryRequest } from 'api/core/registry/service_discovery/v1/service_pb'
import { Process } from 'api/core/registry/service_discovery/v1/models_pb'

type NotificationPlacement = NotificationArgsProps['placement'];

const ServiceRegistryPage: React.FC = () => {
    const [notice, contextHolder] = notification.useNotification();
    const {data, error, isLoading } = useQuery(query, new QueryRequest({
        filter: new Filter({})
    }),{
        select: (data) => {
            let arr: Array<Process> = [];

            Object.keys(data.data).map(k => {
                console.log(data?.data[k])
                arr.push(data?.data[k])
                return data?.data[k]
            })

            return arr
        }
    });

    let table;

    const columns = [
        {
            title: "Service Name",
            dataIndex: "name",
            key: "name"
        },
        {
            title: "Address",
            dataIndex: "ipAddress",
            key: "ipAddress"
        },
        {
            title: "Group",
            dataIndex: "group",
            key: "group"
        },
        {
            title: "Health",
            dataIndex: "healthState",
            key: "healthState"
        },
        {
            title: "Running State",
            dataIndex: "runningState",
            key: "runningState",
        }
    ]

    if ((data) && (!isLoading)) {
        table = <Table dataSource={data} columns={columns} />
    }

    if (isLoading) {
        table =
            <Flex align="center" gap="middle">
                <Spin indicator={<LoadingOutlined style={{ fontSize: 48 }} spin />} />
            </Flex>
    }

    const openNotification = (error: ConnectError, placement: NotificationPlacement) => {
        notice['error']({
            message: error.name,
            description: "failed to retrieve data from the server",
            placement,
        });
    };

    if (error) {
        openNotification(error, 'bottomRight')
    }


    return (
        <div>
            <h2>Registry</h2>
            <div>
                {contextHolder}
                {table}
            </div>
        </div>
    )
}

export default ServiceRegistryPage;