import React from 'react';
import { useQuery } from '@connectrpc/connect-query';
import type { ConnectError } from '@connectrpc/connect';
import { Table, notification, Flex, Spin } from "antd";
import type { NotificationArgsProps } from 'antd';
import { LoadingOutlined } from "@ant-design/icons"

// network messages
import { Any } from "@bufbuild/protobuf";
import { ListRequest } from 'api/core/registry/key_value/v1/service_pb'
import { Value } from 'api/core/registry/key_value/v1/models_pb'
import { list} from 'api/core/registry/key_value/v1/service-KeyValueService_connectquery'

type LocalValue = {
    id: string;
    key: string;
    value: string;
};

type NotificationPlacement = NotificationArgsProps['placement'];

const KeyValuePage: React.FC = () => {
    const [notice, contextHolder] = notification.useNotification();
    const { data, error, isLoading } = useQuery(list, new ListRequest({
        value: Any.pack(new Value({})),
    }), {
        select: (data) => {
            let arr: Array<LocalValue> = [];
            Object.keys(data.values).map(k => {
                let v: Value = new Value()
                v.fromBinary(data?.values[k].value)
                let newValue: LocalValue = {
                    id: k,
                    key: k.replace(`type.googleapis.com/${Value.typeName}-`, ""),
                    value: v.data
                };
                arr.push(newValue);

                return newValue
            })
            return arr
        }
    });

    let table;

    const columns = [
        {
            title: "Key",
            dataIndex: "key",
            key: "id"
        },
        {
            title: "Value",
            dataIndex: "value",
            key: "value"
        }
    ]

    if ((data) && (!isLoading)) {
        table = <Table dataSource={data} columns={columns} />
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

    if (isLoading) {
        table =
            <Flex align="center" gap="middle">
                <Spin indicator={<LoadingOutlined style={{ fontSize: 48 }} spin />} />
            </Flex>
    }

    return (
        <div>
            <h2>Values</h2>
            <div>
                {contextHolder}
                {table}
            </div>
        </div>
    )
}

export default KeyValuePage;