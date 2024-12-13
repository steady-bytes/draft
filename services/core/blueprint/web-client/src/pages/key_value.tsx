import React, {useEffect, useState} from 'react';
import { useMutation, useQuery } from '@connectrpc/connect-query';
import { Any } from "@bufbuild/protobuf";
import { ListRequest } from 'api/core/registry/key_value/v1/service_pb'
import { Value } from 'api/core/registry/key_value/v1/models_pb'
import { list } from 'api/core/registry/key_value/v1/service-KeyValueService_connectquery'

import { Table } from "antd";

type Val = {
    id: string,
    key: string;
    value: string;
};

const KeyValuePage: React.FC = () => {
    let table;

    const { data, error, isLoading } = useQuery(list, new ListRequest({
        value: Any.pack(new Value({}))
    }));

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

    if ((data?.values) && (!isLoading)) {
        let arr: Array<Val> = [];
        
        Object.keys(data?.values).map(k => {
            let v: Value = new Value()
            v.fromBinary(data?.values[k].value) 
            let newValue: Val = {
                id: k,
                key: k.replace("type.googleapis.com/core.registry.key_value.v1.Value-", ""),
                value: v.data
            };
            arr.push(newValue);
        })
        
        table = <Table dataSource={arr} columns={columns} pagination={false}/>
    }

    if (error) {
        console.log("error", error)
    }



    return (
        <div>
            <h2>Key Values</h2>
            <div>
                {table}
            </div>
        </div>
    )
}

export default KeyValuePage;