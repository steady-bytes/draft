# Protocol Documentation
<a name="top"></a>

## Table of Contents

- [gorm/options.proto](#gorm/options.proto)
    - [AutoServerOptions](#gorm.AutoServerOptions)
    - [BelongsToOptions](#gorm.BelongsToOptions)
    - [ExtraField](#gorm.ExtraField)
    - [GormFieldOptions](#gorm.GormFieldOptions)
    - [GormFileOptions](#gorm.GormFileOptions)
    - [GormMessageOptions](#gorm.GormMessageOptions)
    - [GormTag](#gorm.GormTag)
    - [HasManyOptions](#gorm.HasManyOptions)
    - [HasOneOptions](#gorm.HasOneOptions)
    - [ManyToManyOptions](#gorm.ManyToManyOptions)
    - [MethodOptions](#gorm.MethodOptions)
  
    - [File-level Extensions](#gorm/options.proto-extensions)
    - [File-level Extensions](#gorm/options.proto-extensions)
    - [File-level Extensions](#gorm/options.proto-extensions)
    - [File-level Extensions](#gorm/options.proto-extensions)
    - [File-level Extensions](#gorm/options.proto-extensions)
  
- [draft.proto](#draft.proto)
    - [Command](#api.Command)
    - [CreateEventRequest](#api.CreateEventRequest)
    - [CreateEventResponse](#api.CreateEventResponse)
    - [Event](#api.Event)
    - [Output](#api.Output)
    - [ReadAggreageByIDRequest](#api.ReadAggreageByIDRequest)
    - [ReadAggregateByIDRespose](#api.ReadAggregateByIDRespose)
    - [Transaction](#api.Transaction)
  
    - [AggregateKind](#api.AggregateKind)
    - [EventCode](#api.EventCode)
    - [SystemAggregateKind](#api.SystemAggregateKind)
    - [SystemEventCode](#api.SystemEventCode)
  
    - [EventStore](#api.EventStore)
    - [Writer](#api.Writer)
  
- [gorm/types/types.proto](#gorm/types/types.proto)
    - [BigInt](#gorm.types.BigInt)
    - [InetValue](#gorm.types.InetValue)
    - [JSONValue](#gorm.types.JSONValue)
    - [TimeOnly](#gorm.types.TimeOnly)
    - [UUID](#gorm.types.UUID)
    - [UUIDValue](#gorm.types.UUIDValue)
  
- [registry.proto](#registry.proto)
    - [DisconnectRequest](#api.DisconnectRequest)
    - [Disconnected](#api.Disconnected)
    - [Empty](#api.Empty)
    - [Handshake](#api.Handshake)
    - [JournalQueryRequest](#api.JournalQueryRequest)
    - [JournalQueryResponse](#api.JournalQueryResponse)
    - [JournalQueryResponse.ResultEntry](#api.JournalQueryResponse.ResultEntry)
    - [Metadata](#api.Metadata)
    - [MonitorRequest](#api.MonitorRequest)
    - [Process](#api.Process)
    - [ProcessDetails](#api.ProcessDetails)
    - [Query](#api.Query)
    - [RequestHandshake](#api.RequestHandshake)
    - [Token](#api.Token)
  
    - [ProcessHealthState](#api.ProcessHealthState)
    - [ProcessKind](#api.ProcessKind)
    - [ProcessRunningState](#api.ProcessRunningState)
  
    - [Registry](#api.Registry)
  
- [Scalar Value Types](#scalar-value-types)



<a name="gorm/options.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## gorm/options.proto



<a name="gorm.AutoServerOptions"></a>

### AutoServerOptions



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| autogen | [bool](#bool) |  |  |
| txn_middleware | [bool](#bool) |  |  |
| with_tracing | [bool](#bool) |  |  |






<a name="gorm.BelongsToOptions"></a>

### BelongsToOptions



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| foreignkey | [string](#string) |  |  |
| foreignkey_tag | [GormTag](#gorm.GormTag) |  |  |
| association_foreignkey | [string](#string) |  |  |
| association_autoupdate | [bool](#bool) |  |  |
| association_autocreate | [bool](#bool) |  |  |
| association_save_reference | [bool](#bool) |  |  |
| preload | [bool](#bool) |  |  |






<a name="gorm.ExtraField"></a>

### ExtraField



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| type | [string](#string) |  |  |
| name | [string](#string) |  |  |
| tag | [GormTag](#gorm.GormTag) |  |  |
| package | [string](#string) |  |  |






<a name="gorm.GormFieldOptions"></a>

### GormFieldOptions



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| tag | [GormTag](#gorm.GormTag) |  |  |
| drop | [bool](#bool) |  |  |
| has_one | [HasOneOptions](#gorm.HasOneOptions) |  |  |
| belongs_to | [BelongsToOptions](#gorm.BelongsToOptions) |  |  |
| has_many | [HasManyOptions](#gorm.HasManyOptions) |  |  |
| many_to_many | [ManyToManyOptions](#gorm.ManyToManyOptions) |  |  |
| reference_of | [string](#string) |  |  |






<a name="gorm.GormFileOptions"></a>

### GormFileOptions







<a name="gorm.GormMessageOptions"></a>

### GormMessageOptions



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| ormable | [bool](#bool) |  |  |
| include | [ExtraField](#gorm.ExtraField) | repeated |  |
| table | [string](#string) |  |  |
| multi_account | [bool](#bool) |  |  |






<a name="gorm.GormTag"></a>

### GormTag



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| column | [string](#string) |  |  |
| type | [string](#string) |  |  |
| size | [int32](#int32) |  |  |
| precision | [int32](#int32) |  |  |
| primary_key | [bool](#bool) |  |  |
| unique | [bool](#bool) |  |  |
| default | [string](#string) |  |  |
| not_null | [bool](#bool) |  |  |
| auto_increment | [bool](#bool) |  |  |
| index | [string](#string) |  |  |
| unique_index | [string](#string) |  |  |
| embedded | [bool](#bool) |  |  |
| embedded_prefix | [string](#string) |  |  |
| ignore | [bool](#bool) |  |  |
| foreignkey | [string](#string) |  |  |
| association_foreignkey | [string](#string) |  |  |
| many_to_many | [string](#string) |  |  |
| jointable_foreignkey | [string](#string) |  |  |
| association_jointable_foreignkey | [string](#string) |  |  |
| association_autoupdate | [bool](#bool) |  |  |
| association_autocreate | [bool](#bool) |  |  |
| association_save_reference | [bool](#bool) |  |  |
| preload | [bool](#bool) |  |  |






<a name="gorm.HasManyOptions"></a>

### HasManyOptions



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| foreignkey | [string](#string) |  |  |
| foreignkey_tag | [GormTag](#gorm.GormTag) |  |  |
| association_foreignkey | [string](#string) |  |  |
| position_field | [string](#string) |  |  |
| position_field_tag | [GormTag](#gorm.GormTag) |  |  |
| association_autoupdate | [bool](#bool) |  |  |
| association_autocreate | [bool](#bool) |  |  |
| association_save_reference | [bool](#bool) |  |  |
| preload | [bool](#bool) |  |  |
| replace | [bool](#bool) |  |  |
| append | [bool](#bool) |  |  |
| clear | [bool](#bool) |  |  |






<a name="gorm.HasOneOptions"></a>

### HasOneOptions



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| foreignkey | [string](#string) |  |  |
| foreignkey_tag | [GormTag](#gorm.GormTag) |  |  |
| association_foreignkey | [string](#string) |  |  |
| association_autoupdate | [bool](#bool) |  |  |
| association_autocreate | [bool](#bool) |  |  |
| association_save_reference | [bool](#bool) |  |  |
| preload | [bool](#bool) |  |  |
| replace | [bool](#bool) |  |  |
| append | [bool](#bool) |  |  |
| clear | [bool](#bool) |  |  |






<a name="gorm.ManyToManyOptions"></a>

### ManyToManyOptions



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| jointable | [string](#string) |  |  |
| foreignkey | [string](#string) |  |  |
| jointable_foreignkey | [string](#string) |  |  |
| association_foreignkey | [string](#string) |  |  |
| association_jointable_foreignkey | [string](#string) |  |  |
| association_autoupdate | [bool](#bool) |  |  |
| association_autocreate | [bool](#bool) |  |  |
| association_save_reference | [bool](#bool) |  |  |
| preload | [bool](#bool) |  |  |
| replace | [bool](#bool) |  |  |
| append | [bool](#bool) |  |  |
| clear | [bool](#bool) |  |  |






<a name="gorm.MethodOptions"></a>

### MethodOptions



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| object_type | [string](#string) |  |  |





 

 


<a name="gorm/options.proto-extensions"></a>

### File-level Extensions
| Extension | Type | Base | Number | Description |
| --------- | ---- | ---- | ------ | ----------- |
| field | GormFieldOptions | .google.protobuf.FieldOptions | 52119 |  |
| file_opts | GormFileOptions | .google.protobuf.FileOptions | 52119 |  |
| opts | GormMessageOptions | .google.protobuf.MessageOptions | 52119 | ormable will cause orm code to be generated for this message/object |
| method | MethodOptions | .google.protobuf.MethodOptions | 52119 |  |
| server | AutoServerOptions | .google.protobuf.ServiceOptions | 52119 |  |

 

 



<a name="draft.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## draft.proto



<a name="api.Command"></a>

### Command



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  |  |
| arguments | [google.protobuf.Any](#google.protobuf.Any) |  |  |






<a name="api.CreateEventRequest"></a>

### CreateEventRequest
Request, and response messages for the `Event` creation


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| payload | [Event](#api.Event) |  |  |






<a name="api.CreateEventResponse"></a>

### CreateEventResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| result | [Event](#api.Event) |  |  |






<a name="api.Event"></a>

### Event
A generic message type that act&#39;s as a wrapper for all system events. When an event is `Emit`&#39;ed by a producer
the `Event` is stored and forwarded to the correct consumer.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | id - is a uuid to identify each event of the system |
| aggregate_id | [string](#string) |  | aggregate_id - is the identifier of the aggregate the `Event` relates to. |
| transaction_id | [string](#string) |  | transaction_id - is a uuid for each transaction that can be used to string together many differnt events to one executed command |
| data | [string](#string) |  | data - the `data` payload of the event system that can be of any message type the consumer will only be interested in specific types of `Events` from a specifc source. the `event_type` is used as the deserialization/serialization as the type identifier of the `data` value. |
| created_at | [google.protobuf.Timestamp](#google.protobuf.Timestamp) |  | The datetime when the event was stored |
| published_at | [google.protobuf.Timestamp](#google.protobuf.Timestamp) |  | The datetime when the event was published to it&#39;s event stream for processing |
| aggregate_kind | [AggregateKind](#api.AggregateKind) |  |  |
| event_code | [EventCode](#api.EventCode) |  |  |
| side_affect | [bool](#bool) |  | used to determin reply strategy |






<a name="api.Output"></a>

### Output



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| transaction_id | [string](#string) |  |  |
| aggregate_id | [string](#string) |  |  |
| result | [google.protobuf.Any](#google.protobuf.Any) |  |  |






<a name="api.ReadAggreageByIDRequest"></a>

### ReadAggreageByIDRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| aggregate | [string](#string) |  |  |






<a name="api.ReadAggregateByIDRespose"></a>

### ReadAggregateByIDRespose







<a name="api.Transaction"></a>

### Transaction



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| transaction_id | [string](#string) |  |  |
| aggregate_id | [string](#string) |  |  |





 


<a name="api.AggregateKind"></a>

### AggregateKind
APPLICATION AGGREGATES
///////////////////////

| Name | Number | Description |
| ---- | ------ | ----------- |
| INVALID_AGGREGATE | 0 |  |



<a name="api.EventCode"></a>

### EventCode


| Name | Number | Description |
| ---- | ------ | ----------- |
| INVALID_EVENT_CODE | 0 |  |



<a name="api.SystemAggregateKind"></a>

### SystemAggregateKind
Declairs a mapping to the aggregate_type from the `event_code`. The package the event is
imported from is the `AggregateKind`. While also specifiying a group of all Events

| Name | Number | Description |
| ---- | ------ | ----------- |
| INVALID_SYSTEM_AGGREGATE | 0 |  |



<a name="api.SystemEventCode"></a>

### SystemEventCode
EventCode

| Name | Number | Description |
| ---- | ------ | ----------- |
| INVALID_SYSTEM_EVENT_CODE | 0 |  |


 

 


<a name="api.EventStore"></a>

### EventStore
The storage, and routing interface of all `Event`&#39;s in the system.
When an event has been emitted it&#39;s stored, and routed to the correct event stream

| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| Create | [CreateEventRequest](#api.CreateEventRequest) | [CreateEventResponse](#api.CreateEventResponse) | Create - Allows a producer to `Emit` an `Event` making the remaing system aware of a change to the system |


<a name="api.Writer"></a>

### Writer
All system writes can go through two different methods. Exec, or ExecSaga.
`Writes` are segregated from `Reads` following the `CQRS`.

| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| Exec | [Command](#api.Command) | [Output](#api.Output) | Executes a syncrounus command. Meaning the response is expected to contain a result message containing some details. The client will not be expecting the server to response with more data. |
| ExecSaga | [Command](#api.Command) | [Transaction](#api.Transaction) | ExecSaga - Invokes a command using the asyncrounus `Saga` pattern. So a `transaction_id`, and `aggregate_id` are returned, and can then be used by the client to filter streaming results for specific responses |

 



<a name="gorm/types/types.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## gorm/types/types.proto



<a name="gorm.types.BigInt"></a>

### BigInt



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="gorm.types.InetValue"></a>

### InetValue



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="gorm.types.JSONValue"></a>

### JSONValue



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="gorm.types.TimeOnly"></a>

### TimeOnly



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [uint32](#uint32) |  |  |






<a name="gorm.types.UUID"></a>

### UUID



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="gorm.types.UUIDValue"></a>

### UUIDValue



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |





 

 

 

 



<a name="registry.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## registry.proto



<a name="api.DisconnectRequest"></a>

### DisconnectRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| process_id | [string](#string) |  |  |






<a name="api.Disconnected"></a>

### Disconnected



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| process_id | [string](#string) |  |  |






<a name="api.Empty"></a>

### Empty







<a name="api.Handshake"></a>

### Handshake



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| process_id | [string](#string) |  | the process_id is assigned when the join request is successful however it does not mean that the process is registered and running. |
| leader_address | [string](#string) |  | the address the client must stream it&#39;s status messages to |
| token | [Token](#api.Token) |  |  |






<a name="api.JournalQueryRequest"></a>

### JournalQueryRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| look_up | [Query](#api.Query) |  |  |






<a name="api.JournalQueryResponse"></a>

### JournalQueryResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| result | [JournalQueryResponse.ResultEntry](#api.JournalQueryResponse.ResultEntry) | repeated |  |






<a name="api.JournalQueryResponse.ResultEntry"></a>

### JournalQueryResponse.ResultEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [Process](#api.Process) |  |  |






<a name="api.Metadata"></a>

### Metadata



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | id - is a uuid to identify each process of the system |
| key | [string](#string) |  |  |
| value | [string](#string) |  |  |






<a name="api.MonitorRequest"></a>

### MonitorRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| look_up | [Query](#api.Query) |  |  |






<a name="api.Process"></a>

### Process



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | id - is a uuid to identify each process of the system |
| name | [string](#string) |  |  |
| group | [string](#string) |  |  |
| local | [string](#string) |  |  |
| ip_address | [string](#string) |  |  |
| process_kind | [ProcessKind](#api.ProcessKind) |  |  |
| tags | [Metadata](#api.Metadata) | repeated |  |
| joined_time | [google.protobuf.Timestamp](#google.protobuf.Timestamp) |  |  |
| left_time | [google.protobuf.Timestamp](#google.protobuf.Timestamp) |  |  |
| version | [string](#string) |  |  |
| running_state | [ProcessRunningState](#api.ProcessRunningState) |  |  |
| process_health | [ProcessHealthState](#api.ProcessHealthState) |  |  |
| token | [Token](#api.Token) |  |  |






<a name="api.ProcessDetails"></a>

### ProcessDetails



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| process_id | [string](#string) |  |  |
| running_state | [ProcessRunningState](#api.ProcessRunningState) |  |  |
| process_health | [ProcessHealthState](#api.ProcessHealthState) |  |  |






<a name="api.Query"></a>

### Query



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| group | [string](#string) |  |  |
| all | [string](#string) |  |  |






<a name="api.RequestHandshake"></a>

### RequestHandshake



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| payload | [Process](#api.Process) |  |  |






<a name="api.Token"></a>

### Token



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| token | [string](#string) |  |  |
| nonce | [string](#string) |  |  |





 


<a name="api.ProcessHealthState"></a>

### ProcessHealthState


| Name | Number | Description |
| ---- | ------ | ----------- |
| INVALID_PROCESS_HEALTH_STATE | 0 |  |
| PROCESS_HEALTHY | 1 |  |
| PROCESS_UNHEALTHY | 2 |  |



<a name="api.ProcessKind"></a>

### ProcessKind


| Name | Number | Description |
| ---- | ------ | ----------- |
| INVALID_PROCESS_KIND | 0 |  |
| AGGREGATE_PROCESS | 1 |  |
| CONSUMER_PROCESS | 2 |  |
| PROJECTION_PROCESS | 3 |  |
| RPC_PROCESS | 4 |  |
| HTTP_PROCESS | 5 |  |
| DEFAULT_PROCESS | 6 |  |



<a name="api.ProcessRunningState"></a>

### ProcessRunningState


| Name | Number | Description |
| ---- | ------ | ----------- |
| INVALID_PROCESS_RUNNING_STATE | 0 |  |
| PROCESS_STARTING | 1 |  |
| PROCESS_TESTING | 2 |  |
| PROCESS_RUNNING | 3 |  |


 

 


<a name="api.Registry"></a>

### Registry
Process registry
For a process to connect to the registry it&#39;s required to 
`InitiateHandshake`, and then use the `Handshake` details to `Connect` and stream process details to the registry on a set
interval notifiing the registry of the processes state

| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| InitiateHandshake | [RequestHandshake](#api.RequestHandshake) | [Handshake](#api.Handshake) | Initiate the connection process to the registry. After registration is complete, a process will be able to send and receive messages. |
| Connect | [ProcessDetails](#api.ProcessDetails) stream | [Empty](#api.Empty) | The process that has joined the cluster must send connections details of it&#39;s ability to process requests, or perform the business logic it&#39;s supposted to |
| Disconnect | [DisconnectRequest](#api.DisconnectRequest) | [Disconnected](#api.Disconnected) | Disconnect a process from the registry. When a process disconnects. It will no longer be able to send, or receive a message from the system. |
| Monitor | [MonitorRequest](#api.MonitorRequest) | [Process](#api.Process) stream | used by external clients to monitor the status of one, or many processes |
| QuerySystemJournal | [JournalQueryRequest](#api.JournalQueryRequest) | [JournalQueryResponse](#api.JournalQueryResponse) | Query the registries journal of processes |

 



## Scalar Value Types

| .proto Type | Notes | C++ | Java | Python | Go | C# | PHP | Ruby |
| ----------- | ----- | --- | ---- | ------ | -- | -- | --- | ---- |
| <a name="double" /> double |  | double | double | float | float64 | double | float | Float |
| <a name="float" /> float |  | float | float | float | float32 | float | float | Float |
| <a name="int32" /> int32 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint32 instead. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="int64" /> int64 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint64 instead. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="uint32" /> uint32 | Uses variable-length encoding. | uint32 | int | int/long | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="uint64" /> uint64 | Uses variable-length encoding. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum or Fixnum (as required) |
| <a name="sint32" /> sint32 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int32s. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sint64" /> sint64 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int64s. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="fixed32" /> fixed32 | Always four bytes. More efficient than uint32 if values are often greater than 2^28. | uint32 | int | int | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="fixed64" /> fixed64 | Always eight bytes. More efficient than uint64 if values are often greater than 2^56. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum |
| <a name="sfixed32" /> sfixed32 | Always four bytes. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sfixed64" /> sfixed64 | Always eight bytes. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="bool" /> bool |  | bool | boolean | boolean | bool | bool | boolean | TrueClass/FalseClass |
| <a name="string" /> string | A string must always contain UTF-8 encoded or 7-bit ASCII text. | string | String | str/unicode | string | string | string | String (UTF-8) |
| <a name="bytes" /> bytes | May contain any arbitrary sequence of bytes. | string | ByteString | str | []byte | ByteString | string | String (ASCII-8BIT) |

