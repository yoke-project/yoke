import datetime

from google.protobuf import timestamp_pb2 as _timestamp_pb2
from yoke.interface.v1 import events_pb2 as _events_pb2
from yoke.interface.v1 import records_pb2 as _records_pb2
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Request(_message.Message):
    __slots__ = ("version", "authenticate", "read", "subscribe", "confirm", "command", "query", "stream_start", "stream_stop", "stream_subscribe", "stream_unsubscribe", "reclaim")
    VERSION_FIELD_NUMBER: _ClassVar[int]
    AUTHENTICATE_FIELD_NUMBER: _ClassVar[int]
    READ_FIELD_NUMBER: _ClassVar[int]
    SUBSCRIBE_FIELD_NUMBER: _ClassVar[int]
    CONFIRM_FIELD_NUMBER: _ClassVar[int]
    COMMAND_FIELD_NUMBER: _ClassVar[int]
    QUERY_FIELD_NUMBER: _ClassVar[int]
    STREAM_START_FIELD_NUMBER: _ClassVar[int]
    STREAM_STOP_FIELD_NUMBER: _ClassVar[int]
    STREAM_SUBSCRIBE_FIELD_NUMBER: _ClassVar[int]
    STREAM_UNSUBSCRIBE_FIELD_NUMBER: _ClassVar[int]
    RECLAIM_FIELD_NUMBER: _ClassVar[int]
    version: int
    authenticate: Authenticate
    read: Read
    subscribe: Subscribe
    confirm: Confirm
    command: Command
    query: Question
    stream_start: UnitStream
    stream_stop: UnitStream
    stream_subscribe: UnitStream
    stream_unsubscribe: StreamRelease
    reclaim: Reclaim
    def __init__(self, version: _Optional[int] = ..., authenticate: _Optional[_Union[Authenticate, _Mapping]] = ..., read: _Optional[_Union[Read, _Mapping]] = ..., subscribe: _Optional[_Union[Subscribe, _Mapping]] = ..., confirm: _Optional[_Union[Confirm, _Mapping]] = ..., command: _Optional[_Union[Command, _Mapping]] = ..., query: _Optional[_Union[Question, _Mapping]] = ..., stream_start: _Optional[_Union[UnitStream, _Mapping]] = ..., stream_stop: _Optional[_Union[UnitStream, _Mapping]] = ..., stream_subscribe: _Optional[_Union[UnitStream, _Mapping]] = ..., stream_unsubscribe: _Optional[_Union[StreamRelease, _Mapping]] = ..., reclaim: _Optional[_Union[Reclaim, _Mapping]] = ...) -> None: ...

class Authenticate(_message.Message):
    __slots__ = ("password", "token")
    class Password(_message.Message):
        __slots__ = ("account", "secret")
        ACCOUNT_FIELD_NUMBER: _ClassVar[int]
        SECRET_FIELD_NUMBER: _ClassVar[int]
        account: str
        secret: str
        def __init__(self, account: _Optional[str] = ..., secret: _Optional[str] = ...) -> None: ...
    PASSWORD_FIELD_NUMBER: _ClassVar[int]
    TOKEN_FIELD_NUMBER: _ClassVar[int]
    password: Authenticate.Password
    token: str
    def __init__(self, password: _Optional[_Union[Authenticate.Password, _Mapping]] = ..., token: _Optional[str] = ...) -> None: ...

class Read(_message.Message):
    __slots__ = ("kind", "identity")
    KIND_FIELD_NUMBER: _ClassVar[int]
    IDENTITY_FIELD_NUMBER: _ClassVar[int]
    kind: str
    identity: str
    def __init__(self, kind: _Optional[str] = ..., identity: _Optional[str] = ...) -> None: ...

class Subscribe(_message.Message):
    __slots__ = ("filter",)
    FILTER_FIELD_NUMBER: _ClassVar[int]
    filter: _events_pb2.Filter
    def __init__(self, filter: _Optional[_Union[_events_pb2.Filter, _Mapping]] = ...) -> None: ...

class Confirm(_message.Message):
    __slots__ = ("subscription", "sequence")
    SUBSCRIPTION_FIELD_NUMBER: _ClassVar[int]
    SEQUENCE_FIELD_NUMBER: _ClassVar[int]
    subscription: str
    sequence: int
    def __init__(self, subscription: _Optional[str] = ..., sequence: _Optional[int] = ...) -> None: ...

class Command(_message.Message):
    __slots__ = ("unit", "type", "payload")
    UNIT_FIELD_NUMBER: _ClassVar[int]
    TYPE_FIELD_NUMBER: _ClassVar[int]
    PAYLOAD_FIELD_NUMBER: _ClassVar[int]
    unit: str
    type: str
    payload: bytes
    def __init__(self, unit: _Optional[str] = ..., type: _Optional[str] = ..., payload: _Optional[bytes] = ...) -> None: ...

class Question(_message.Message):
    __slots__ = ("unit", "type", "payload")
    UNIT_FIELD_NUMBER: _ClassVar[int]
    TYPE_FIELD_NUMBER: _ClassVar[int]
    PAYLOAD_FIELD_NUMBER: _ClassVar[int]
    unit: str
    type: str
    payload: bytes
    def __init__(self, unit: _Optional[str] = ..., type: _Optional[str] = ..., payload: _Optional[bytes] = ...) -> None: ...

class UnitStream(_message.Message):
    __slots__ = ("unit", "stream")
    UNIT_FIELD_NUMBER: _ClassVar[int]
    STREAM_FIELD_NUMBER: _ClassVar[int]
    unit: str
    stream: str
    def __init__(self, unit: _Optional[str] = ..., stream: _Optional[str] = ...) -> None: ...

class StreamRelease(_message.Message):
    __slots__ = ("delivery",)
    DELIVERY_FIELD_NUMBER: _ClassVar[int]
    delivery: str
    def __init__(self, delivery: _Optional[str] = ...) -> None: ...

class Reclaim(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class Response(_message.Message):
    __slots__ = ("authenticate", "read", "subscribe", "confirm", "command", "query", "stream_start", "stream_stop", "stream_subscribe", "stream_unsubscribe", "reclaim")
    AUTHENTICATE_FIELD_NUMBER: _ClassVar[int]
    READ_FIELD_NUMBER: _ClassVar[int]
    SUBSCRIBE_FIELD_NUMBER: _ClassVar[int]
    CONFIRM_FIELD_NUMBER: _ClassVar[int]
    COMMAND_FIELD_NUMBER: _ClassVar[int]
    QUERY_FIELD_NUMBER: _ClassVar[int]
    STREAM_START_FIELD_NUMBER: _ClassVar[int]
    STREAM_STOP_FIELD_NUMBER: _ClassVar[int]
    STREAM_SUBSCRIBE_FIELD_NUMBER: _ClassVar[int]
    STREAM_UNSUBSCRIBE_FIELD_NUMBER: _ClassVar[int]
    RECLAIM_FIELD_NUMBER: _ClassVar[int]
    authenticate: Authenticated
    read: _records_pb2.Records
    subscribe: Subscribed
    confirm: Confirmed
    command: Acknowledged
    query: Answered
    stream_start: Acknowledged
    stream_stop: Acknowledged
    stream_subscribe: Delivering
    stream_unsubscribe: Released
    reclaim: Reclaimed
    def __init__(self, authenticate: _Optional[_Union[Authenticated, _Mapping]] = ..., read: _Optional[_Union[_records_pb2.Records, _Mapping]] = ..., subscribe: _Optional[_Union[Subscribed, _Mapping]] = ..., confirm: _Optional[_Union[Confirmed, _Mapping]] = ..., command: _Optional[_Union[Acknowledged, _Mapping]] = ..., query: _Optional[_Union[Answered, _Mapping]] = ..., stream_start: _Optional[_Union[Acknowledged, _Mapping]] = ..., stream_stop: _Optional[_Union[Acknowledged, _Mapping]] = ..., stream_subscribe: _Optional[_Union[Delivering, _Mapping]] = ..., stream_unsubscribe: _Optional[_Union[Released, _Mapping]] = ..., reclaim: _Optional[_Union[Reclaimed, _Mapping]] = ...) -> None: ...

class Authenticated(_message.Message):
    __slots__ = ("account", "token", "expires")
    ACCOUNT_FIELD_NUMBER: _ClassVar[int]
    TOKEN_FIELD_NUMBER: _ClassVar[int]
    EXPIRES_FIELD_NUMBER: _ClassVar[int]
    account: str
    token: str
    expires: _timestamp_pb2.Timestamp
    def __init__(self, account: _Optional[str] = ..., token: _Optional[str] = ..., expires: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class Subscribed(_message.Message):
    __slots__ = ("snapshot", "event", "overflow")
    SNAPSHOT_FIELD_NUMBER: _ClassVar[int]
    EVENT_FIELD_NUMBER: _ClassVar[int]
    OVERFLOW_FIELD_NUMBER: _ClassVar[int]
    snapshot: _records_pb2.Snapshot
    event: _events_pb2.Event
    overflow: _records_pb2.Snapshot
    def __init__(self, snapshot: _Optional[_Union[_records_pb2.Snapshot, _Mapping]] = ..., event: _Optional[_Union[_events_pb2.Event, _Mapping]] = ..., overflow: _Optional[_Union[_records_pb2.Snapshot, _Mapping]] = ...) -> None: ...

class Confirmed(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class Acknowledged(_message.Message):
    __slots__ = ("outcome", "line")
    class Outcome(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
        __slots__ = ()
        OUTCOME_UNSPECIFIED: _ClassVar[Acknowledged.Outcome]
        OUTCOME_ACCEPTED: _ClassVar[Acknowledged.Outcome]
        OUTCOME_DONE: _ClassVar[Acknowledged.Outcome]
        OUTCOME_FAILED: _ClassVar[Acknowledged.Outcome]
    OUTCOME_UNSPECIFIED: Acknowledged.Outcome
    OUTCOME_ACCEPTED: Acknowledged.Outcome
    OUTCOME_DONE: Acknowledged.Outcome
    OUTCOME_FAILED: Acknowledged.Outcome
    OUTCOME_FIELD_NUMBER: _ClassVar[int]
    LINE_FIELD_NUMBER: _ClassVar[int]
    outcome: Acknowledged.Outcome
    line: str
    def __init__(self, outcome: _Optional[_Union[Acknowledged.Outcome, str]] = ..., line: _Optional[str] = ...) -> None: ...

class Answered(_message.Message):
    __slots__ = ("payload",)
    PAYLOAD_FIELD_NUMBER: _ClassVar[int]
    payload: bytes
    def __init__(self, payload: _Optional[bytes] = ...) -> None: ...

class Delivering(_message.Message):
    __slots__ = ("delivery", "socket", "connection", "path", "flowing")
    DELIVERY_FIELD_NUMBER: _ClassVar[int]
    SOCKET_FIELD_NUMBER: _ClassVar[int]
    CONNECTION_FIELD_NUMBER: _ClassVar[int]
    PATH_FIELD_NUMBER: _ClassVar[int]
    FLOWING_FIELD_NUMBER: _ClassVar[int]
    delivery: str
    socket: str
    connection: bool
    path: str
    flowing: bool
    def __init__(self, delivery: _Optional[str] = ..., socket: _Optional[str] = ..., connection: _Optional[bool] = ..., path: _Optional[str] = ..., flowing: _Optional[bool] = ...) -> None: ...

class Released(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class Reclaimed(_message.Message):
    __slots__ = ("channel", "changed")
    CHANNEL_FIELD_NUMBER: _ClassVar[int]
    CHANGED_FIELD_NUMBER: _ClassVar[int]
    channel: _records_pb2.ChannelRecord
    changed: bool
    def __init__(self, channel: _Optional[_Union[_records_pb2.ChannelRecord, _Mapping]] = ..., changed: _Optional[bool] = ...) -> None: ...
