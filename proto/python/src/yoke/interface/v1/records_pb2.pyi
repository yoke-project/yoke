import datetime

from google.protobuf import timestamp_pb2 as _timestamp_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Record(_message.Message):
    __slots__ = ("instance", "unit", "channel")
    INSTANCE_FIELD_NUMBER: _ClassVar[int]
    UNIT_FIELD_NUMBER: _ClassVar[int]
    CHANNEL_FIELD_NUMBER: _ClassVar[int]
    instance: InstanceRecord
    unit: UnitRecord
    channel: ChannelRecord
    def __init__(self, instance: _Optional[_Union[InstanceRecord, _Mapping]] = ..., unit: _Optional[_Union[UnitRecord, _Mapping]] = ..., channel: _Optional[_Union[ChannelRecord, _Mapping]] = ...) -> None: ...

class InstanceRecord(_message.Message):
    __slots__ = ("ready", "stopping", "since")
    READY_FIELD_NUMBER: _ClassVar[int]
    STOPPING_FIELD_NUMBER: _ClassVar[int]
    SINCE_FIELD_NUMBER: _ClassVar[int]
    ready: bool
    stopping: bool
    since: _timestamp_pb2.Timestamp
    def __init__(self, ready: _Optional[bool] = ..., stopping: _Optional[bool] = ..., since: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class UnitRecord(_message.Message):
    __slots__ = ("declared", "observed", "addressed")
    class Declared(_message.Message):
        __slots__ = ("identity", "kind")
        IDENTITY_FIELD_NUMBER: _ClassVar[int]
        KIND_FIELD_NUMBER: _ClassVar[int]
        identity: str
        kind: str
        def __init__(self, identity: _Optional[str] = ..., kind: _Optional[str] = ...) -> None: ...
    class Observed(_message.Message):
        __slots__ = ("state", "incarnation", "since", "condition", "streams")
        STATE_FIELD_NUMBER: _ClassVar[int]
        INCARNATION_FIELD_NUMBER: _ClassVar[int]
        SINCE_FIELD_NUMBER: _ClassVar[int]
        CONDITION_FIELD_NUMBER: _ClassVar[int]
        STREAMS_FIELD_NUMBER: _ClassVar[int]
        state: str
        incarnation: int
        since: _timestamp_pb2.Timestamp
        condition: Condition
        streams: _containers.RepeatedScalarFieldContainer[str]
        def __init__(self, state: _Optional[str] = ..., incarnation: _Optional[int] = ..., since: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., condition: _Optional[_Union[Condition, _Mapping]] = ..., streams: _Optional[_Iterable[str]] = ...) -> None: ...
    class Addressed(_message.Message):
        __slots__ = ("streams", "commands", "queries")
        STREAMS_FIELD_NUMBER: _ClassVar[int]
        COMMANDS_FIELD_NUMBER: _ClassVar[int]
        QUERIES_FIELD_NUMBER: _ClassVar[int]
        streams: _containers.RepeatedScalarFieldContainer[str]
        commands: _containers.RepeatedScalarFieldContainer[str]
        queries: _containers.RepeatedScalarFieldContainer[str]
        def __init__(self, streams: _Optional[_Iterable[str]] = ..., commands: _Optional[_Iterable[str]] = ..., queries: _Optional[_Iterable[str]] = ...) -> None: ...
    DECLARED_FIELD_NUMBER: _ClassVar[int]
    OBSERVED_FIELD_NUMBER: _ClassVar[int]
    ADDRESSED_FIELD_NUMBER: _ClassVar[int]
    declared: UnitRecord.Declared
    observed: UnitRecord.Observed
    addressed: UnitRecord.Addressed
    def __init__(self, declared: _Optional[_Union[UnitRecord.Declared, _Mapping]] = ..., observed: _Optional[_Union[UnitRecord.Observed, _Mapping]] = ..., addressed: _Optional[_Union[UnitRecord.Addressed, _Mapping]] = ...) -> None: ...

class Condition(_message.Message):
    __slots__ = ("grade", "line", "since")
    GRADE_FIELD_NUMBER: _ClassVar[int]
    LINE_FIELD_NUMBER: _ClassVar[int]
    SINCE_FIELD_NUMBER: _ClassVar[int]
    grade: int
    line: str
    since: _timestamp_pb2.Timestamp
    def __init__(self, grade: _Optional[int] = ..., line: _Optional[str] = ..., since: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class ChannelRecord(_message.Message):
    __slots__ = ("declared", "observed")
    class Declared(_message.Message):
        __slots__ = ("name", "projection", "address_class", "clients")
        NAME_FIELD_NUMBER: _ClassVar[int]
        PROJECTION_FIELD_NUMBER: _ClassVar[int]
        ADDRESS_CLASS_FIELD_NUMBER: _ClassVar[int]
        CLIENTS_FIELD_NUMBER: _ClassVar[int]
        name: str
        projection: str
        address_class: str
        clients: str
        def __init__(self, name: _Optional[str] = ..., projection: _Optional[str] = ..., address_class: _Optional[str] = ..., clients: _Optional[str] = ...) -> None: ...
    class Observed(_message.Message):
        __slots__ = ("attached", "client", "suspended", "grade", "by", "reason")
        ATTACHED_FIELD_NUMBER: _ClassVar[int]
        CLIENT_FIELD_NUMBER: _ClassVar[int]
        SUSPENDED_FIELD_NUMBER: _ClassVar[int]
        GRADE_FIELD_NUMBER: _ClassVar[int]
        BY_FIELD_NUMBER: _ClassVar[int]
        REASON_FIELD_NUMBER: _ClassVar[int]
        attached: bool
        client: str
        suspended: bool
        grade: str
        by: str
        reason: str
        def __init__(self, attached: _Optional[bool] = ..., client: _Optional[str] = ..., suspended: _Optional[bool] = ..., grade: _Optional[str] = ..., by: _Optional[str] = ..., reason: _Optional[str] = ...) -> None: ...
    DECLARED_FIELD_NUMBER: _ClassVar[int]
    OBSERVED_FIELD_NUMBER: _ClassVar[int]
    declared: ChannelRecord.Declared
    observed: ChannelRecord.Observed
    def __init__(self, declared: _Optional[_Union[ChannelRecord.Declared, _Mapping]] = ..., observed: _Optional[_Union[ChannelRecord.Observed, _Mapping]] = ...) -> None: ...

class Snapshot(_message.Message):
    __slots__ = ("at", "records")
    AT_FIELD_NUMBER: _ClassVar[int]
    RECORDS_FIELD_NUMBER: _ClassVar[int]
    at: int
    records: _containers.RepeatedCompositeFieldContainer[Record]
    def __init__(self, at: _Optional[int] = ..., records: _Optional[_Iterable[_Union[Record, _Mapping]]] = ...) -> None: ...

class Records(_message.Message):
    __slots__ = ("records",)
    RECORDS_FIELD_NUMBER: _ClassVar[int]
    records: _containers.RepeatedCompositeFieldContainer[Record]
    def __init__(self, records: _Optional[_Iterable[_Union[Record, _Mapping]]] = ...) -> None: ...
