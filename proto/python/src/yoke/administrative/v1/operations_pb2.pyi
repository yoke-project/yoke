import datetime

from google.protobuf import duration_pb2 as _duration_pb2
from google.protobuf import timestamp_pb2 as _timestamp_pb2
from yoke.administrative.v1 import events_pb2 as _events_pb2
from yoke.administrative.v1 import records_pb2 as _records_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Request(_message.Message):
    __slots__ = ("version", "plugin_enable", "plugin_disable", "plugin_grant", "plugin_withdraw", "unit_start", "unit_stop", "unit_restart", "unit_stream_start", "unit_stream_stop", "unit_retention_set", "unit_retention_clear", "unit_ask", "read", "log_query", "log_follow", "subscribe")
    VERSION_FIELD_NUMBER: _ClassVar[int]
    PLUGIN_ENABLE_FIELD_NUMBER: _ClassVar[int]
    PLUGIN_DISABLE_FIELD_NUMBER: _ClassVar[int]
    PLUGIN_GRANT_FIELD_NUMBER: _ClassVar[int]
    PLUGIN_WITHDRAW_FIELD_NUMBER: _ClassVar[int]
    UNIT_START_FIELD_NUMBER: _ClassVar[int]
    UNIT_STOP_FIELD_NUMBER: _ClassVar[int]
    UNIT_RESTART_FIELD_NUMBER: _ClassVar[int]
    UNIT_STREAM_START_FIELD_NUMBER: _ClassVar[int]
    UNIT_STREAM_STOP_FIELD_NUMBER: _ClassVar[int]
    UNIT_RETENTION_SET_FIELD_NUMBER: _ClassVar[int]
    UNIT_RETENTION_CLEAR_FIELD_NUMBER: _ClassVar[int]
    UNIT_ASK_FIELD_NUMBER: _ClassVar[int]
    READ_FIELD_NUMBER: _ClassVar[int]
    LOG_QUERY_FIELD_NUMBER: _ClassVar[int]
    LOG_FOLLOW_FIELD_NUMBER: _ClassVar[int]
    SUBSCRIBE_FIELD_NUMBER: _ClassVar[int]
    version: int
    plugin_enable: PluginPolicy
    plugin_disable: PluginPolicy
    plugin_grant: PluginGrant
    plugin_withdraw: PluginGrant
    unit_start: UnitAct
    unit_stop: UnitAct
    unit_restart: UnitAct
    unit_stream_start: UnitStream
    unit_stream_stop: UnitStream
    unit_retention_set: UnitRetention
    unit_retention_clear: UnitAct
    unit_ask: UnitAsk
    read: Read
    log_query: LogQuery
    log_follow: LogFollow
    subscribe: Subscribe
    def __init__(self, version: _Optional[int] = ..., plugin_enable: _Optional[_Union[PluginPolicy, _Mapping]] = ..., plugin_disable: _Optional[_Union[PluginPolicy, _Mapping]] = ..., plugin_grant: _Optional[_Union[PluginGrant, _Mapping]] = ..., plugin_withdraw: _Optional[_Union[PluginGrant, _Mapping]] = ..., unit_start: _Optional[_Union[UnitAct, _Mapping]] = ..., unit_stop: _Optional[_Union[UnitAct, _Mapping]] = ..., unit_restart: _Optional[_Union[UnitAct, _Mapping]] = ..., unit_stream_start: _Optional[_Union[UnitStream, _Mapping]] = ..., unit_stream_stop: _Optional[_Union[UnitStream, _Mapping]] = ..., unit_retention_set: _Optional[_Union[UnitRetention, _Mapping]] = ..., unit_retention_clear: _Optional[_Union[UnitAct, _Mapping]] = ..., unit_ask: _Optional[_Union[UnitAsk, _Mapping]] = ..., read: _Optional[_Union[Read, _Mapping]] = ..., log_query: _Optional[_Union[LogQuery, _Mapping]] = ..., log_follow: _Optional[_Union[LogFollow, _Mapping]] = ..., subscribe: _Optional[_Union[Subscribe, _Mapping]] = ...) -> None: ...

class PluginPolicy(_message.Message):
    __slots__ = ("plugin",)
    PLUGIN_FIELD_NUMBER: _ClassVar[int]
    plugin: str
    def __init__(self, plugin: _Optional[str] = ...) -> None: ...

class PluginGrant(_message.Message):
    __slots__ = ("plugin", "capability")
    PLUGIN_FIELD_NUMBER: _ClassVar[int]
    CAPABILITY_FIELD_NUMBER: _ClassVar[int]
    plugin: str
    capability: str
    def __init__(self, plugin: _Optional[str] = ..., capability: _Optional[str] = ...) -> None: ...

class UnitAct(_message.Message):
    __slots__ = ("unit",)
    UNIT_FIELD_NUMBER: _ClassVar[int]
    unit: str
    def __init__(self, unit: _Optional[str] = ...) -> None: ...

class UnitStream(_message.Message):
    __slots__ = ("unit", "stream")
    UNIT_FIELD_NUMBER: _ClassVar[int]
    STREAM_FIELD_NUMBER: _ClassVar[int]
    unit: str
    stream: str
    def __init__(self, unit: _Optional[str] = ..., stream: _Optional[str] = ...) -> None: ...

class UnitRetention(_message.Message):
    __slots__ = ("unit", "age", "bytes", "entries")
    UNIT_FIELD_NUMBER: _ClassVar[int]
    AGE_FIELD_NUMBER: _ClassVar[int]
    BYTES_FIELD_NUMBER: _ClassVar[int]
    ENTRIES_FIELD_NUMBER: _ClassVar[int]
    unit: str
    age: _duration_pb2.Duration
    bytes: int
    entries: int
    def __init__(self, unit: _Optional[str] = ..., age: _Optional[_Union[datetime.timedelta, _duration_pb2.Duration, _Mapping]] = ..., bytes: _Optional[int] = ..., entries: _Optional[int] = ...) -> None: ...

class UnitAsk(_message.Message):
    __slots__ = ("unit", "type", "question")
    UNIT_FIELD_NUMBER: _ClassVar[int]
    TYPE_FIELD_NUMBER: _ClassVar[int]
    QUESTION_FIELD_NUMBER: _ClassVar[int]
    unit: str
    type: str
    question: bytes
    def __init__(self, unit: _Optional[str] = ..., type: _Optional[str] = ..., question: _Optional[bytes] = ...) -> None: ...

class Read(_message.Message):
    __slots__ = ("kind", "identity")
    KIND_FIELD_NUMBER: _ClassVar[int]
    IDENTITY_FIELD_NUMBER: _ClassVar[int]
    kind: str
    identity: str
    def __init__(self, kind: _Optional[str] = ..., identity: _Optional[str] = ...) -> None: ...

class LogQuery(_message.Message):
    __slots__ = ("unit", "incarnation", "until", "floor", "cursor")
    UNIT_FIELD_NUMBER: _ClassVar[int]
    INCARNATION_FIELD_NUMBER: _ClassVar[int]
    FROM_FIELD_NUMBER: _ClassVar[int]
    UNTIL_FIELD_NUMBER: _ClassVar[int]
    FLOOR_FIELD_NUMBER: _ClassVar[int]
    CURSOR_FIELD_NUMBER: _ClassVar[int]
    unit: str
    incarnation: int
    until: _timestamp_pb2.Timestamp
    floor: int
    cursor: int
    def __init__(self, unit: _Optional[str] = ..., incarnation: _Optional[int] = ..., until: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., floor: _Optional[int] = ..., cursor: _Optional[int] = ..., **kwargs) -> None: ...

class LogFollow(_message.Message):
    __slots__ = ("unit", "incarnation", "floor", "cursor")
    UNIT_FIELD_NUMBER: _ClassVar[int]
    INCARNATION_FIELD_NUMBER: _ClassVar[int]
    FLOOR_FIELD_NUMBER: _ClassVar[int]
    CURSOR_FIELD_NUMBER: _ClassVar[int]
    unit: str
    incarnation: int
    floor: int
    cursor: int
    def __init__(self, unit: _Optional[str] = ..., incarnation: _Optional[int] = ..., floor: _Optional[int] = ..., cursor: _Optional[int] = ...) -> None: ...

class Subscribe(_message.Message):
    __slots__ = ("filter",)
    FILTER_FIELD_NUMBER: _ClassVar[int]
    filter: _events_pb2.Filter
    def __init__(self, filter: _Optional[_Union[_events_pb2.Filter, _Mapping]] = ...) -> None: ...

class Response(_message.Message):
    __slots__ = ("plugin_enable", "plugin_disable", "plugin_grant", "plugin_withdraw", "unit_start", "unit_stop", "unit_restart", "unit_stream_start", "unit_stream_stop", "unit_retention_set", "unit_retention_clear", "unit_ask", "read", "log_query", "log_follow", "subscribe")
    PLUGIN_ENABLE_FIELD_NUMBER: _ClassVar[int]
    PLUGIN_DISABLE_FIELD_NUMBER: _ClassVar[int]
    PLUGIN_GRANT_FIELD_NUMBER: _ClassVar[int]
    PLUGIN_WITHDRAW_FIELD_NUMBER: _ClassVar[int]
    UNIT_START_FIELD_NUMBER: _ClassVar[int]
    UNIT_STOP_FIELD_NUMBER: _ClassVar[int]
    UNIT_RESTART_FIELD_NUMBER: _ClassVar[int]
    UNIT_STREAM_START_FIELD_NUMBER: _ClassVar[int]
    UNIT_STREAM_STOP_FIELD_NUMBER: _ClassVar[int]
    UNIT_RETENTION_SET_FIELD_NUMBER: _ClassVar[int]
    UNIT_RETENTION_CLEAR_FIELD_NUMBER: _ClassVar[int]
    UNIT_ASK_FIELD_NUMBER: _ClassVar[int]
    READ_FIELD_NUMBER: _ClassVar[int]
    LOG_QUERY_FIELD_NUMBER: _ClassVar[int]
    LOG_FOLLOW_FIELD_NUMBER: _ClassVar[int]
    SUBSCRIBE_FIELD_NUMBER: _ClassVar[int]
    plugin_enable: Changed
    plugin_disable: Changed
    plugin_grant: Changed
    plugin_withdraw: Changed
    unit_start: Changed
    unit_stop: Changed
    unit_restart: Changed
    unit_stream_start: Changed
    unit_stream_stop: Changed
    unit_retention_set: Changed
    unit_retention_clear: Changed
    unit_ask: Asked
    read: _records_pb2.Records
    log_query: LogPage
    log_follow: Followed
    subscribe: Subscribed
    def __init__(self, plugin_enable: _Optional[_Union[Changed, _Mapping]] = ..., plugin_disable: _Optional[_Union[Changed, _Mapping]] = ..., plugin_grant: _Optional[_Union[Changed, _Mapping]] = ..., plugin_withdraw: _Optional[_Union[Changed, _Mapping]] = ..., unit_start: _Optional[_Union[Changed, _Mapping]] = ..., unit_stop: _Optional[_Union[Changed, _Mapping]] = ..., unit_restart: _Optional[_Union[Changed, _Mapping]] = ..., unit_stream_start: _Optional[_Union[Changed, _Mapping]] = ..., unit_stream_stop: _Optional[_Union[Changed, _Mapping]] = ..., unit_retention_set: _Optional[_Union[Changed, _Mapping]] = ..., unit_retention_clear: _Optional[_Union[Changed, _Mapping]] = ..., unit_ask: _Optional[_Union[Asked, _Mapping]] = ..., read: _Optional[_Union[_records_pb2.Records, _Mapping]] = ..., log_query: _Optional[_Union[LogPage, _Mapping]] = ..., log_follow: _Optional[_Union[Followed, _Mapping]] = ..., subscribe: _Optional[_Union[Subscribed, _Mapping]] = ...) -> None: ...

class Changed(_message.Message):
    __slots__ = ("previously", "effective", "consequences")
    class Effective(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
        __slots__ = ()
        EFFECTIVE_UNSPECIFIED: _ClassVar[Changed.Effective]
        EFFECTIVE_IMMEDIATELY: _ClassVar[Changed.Effective]
        EFFECTIVE_AT_NEXT_ADMISSION: _ClassVar[Changed.Effective]
    EFFECTIVE_UNSPECIFIED: Changed.Effective
    EFFECTIVE_IMMEDIATELY: Changed.Effective
    EFFECTIVE_AT_NEXT_ADMISSION: Changed.Effective
    PREVIOUSLY_FIELD_NUMBER: _ClassVar[int]
    EFFECTIVE_FIELD_NUMBER: _ClassVar[int]
    CONSEQUENCES_FIELD_NUMBER: _ClassVar[int]
    previously: Previously
    effective: Changed.Effective
    consequences: _containers.RepeatedCompositeFieldContainer[Consequence]
    def __init__(self, previously: _Optional[_Union[Previously, _Mapping]] = ..., effective: _Optional[_Union[Changed.Effective, str]] = ..., consequences: _Optional[_Iterable[_Union[Consequence, _Mapping]]] = ...) -> None: ...

class Previously(_message.Message):
    __slots__ = ("enabled", "granted", "state", "retention")
    ENABLED_FIELD_NUMBER: _ClassVar[int]
    GRANTED_FIELD_NUMBER: _ClassVar[int]
    STATE_FIELD_NUMBER: _ClassVar[int]
    RETENTION_FIELD_NUMBER: _ClassVar[int]
    enabled: bool
    granted: bool
    state: str
    retention: Retention
    def __init__(self, enabled: _Optional[bool] = ..., granted: _Optional[bool] = ..., state: _Optional[str] = ..., retention: _Optional[_Union[Retention, _Mapping]] = ...) -> None: ...

class Retention(_message.Message):
    __slots__ = ("age", "bytes", "entries")
    AGE_FIELD_NUMBER: _ClassVar[int]
    BYTES_FIELD_NUMBER: _ClassVar[int]
    ENTRIES_FIELD_NUMBER: _ClassVar[int]
    age: _duration_pb2.Duration
    bytes: int
    entries: int
    def __init__(self, age: _Optional[_Union[datetime.timedelta, _duration_pb2.Duration, _Mapping]] = ..., bytes: _Optional[int] = ..., entries: _Optional[int] = ...) -> None: ...

class Consequence(_message.Message):
    __slots__ = ("unit", "incarnation", "what")
    UNIT_FIELD_NUMBER: _ClassVar[int]
    INCARNATION_FIELD_NUMBER: _ClassVar[int]
    WHAT_FIELD_NUMBER: _ClassVar[int]
    unit: str
    incarnation: int
    what: str
    def __init__(self, unit: _Optional[str] = ..., incarnation: _Optional[int] = ..., what: _Optional[str] = ...) -> None: ...

class Asked(_message.Message):
    __slots__ = ("answer",)
    ANSWER_FIELD_NUMBER: _ClassVar[int]
    answer: bytes
    def __init__(self, answer: _Optional[bytes] = ...) -> None: ...

class LogPage(_message.Message):
    __slots__ = ("entries", "next", "resumed")
    ENTRIES_FIELD_NUMBER: _ClassVar[int]
    NEXT_FIELD_NUMBER: _ClassVar[int]
    RESUMED_FIELD_NUMBER: _ClassVar[int]
    entries: _containers.RepeatedCompositeFieldContainer[LogEntry]
    next: int
    resumed: bool
    def __init__(self, entries: _Optional[_Iterable[_Union[LogEntry, _Mapping]]] = ..., next: _Optional[int] = ..., resumed: _Optional[bool] = ...) -> None: ...

class LogEntry(_message.Message):
    __slots__ = ("seq", "at", "unit", "incarnation", "source", "severity", "type", "subject_kind", "subject_id", "actor", "cause", "message", "detail")
    SEQ_FIELD_NUMBER: _ClassVar[int]
    AT_FIELD_NUMBER: _ClassVar[int]
    UNIT_FIELD_NUMBER: _ClassVar[int]
    INCARNATION_FIELD_NUMBER: _ClassVar[int]
    SOURCE_FIELD_NUMBER: _ClassVar[int]
    SEVERITY_FIELD_NUMBER: _ClassVar[int]
    TYPE_FIELD_NUMBER: _ClassVar[int]
    SUBJECT_KIND_FIELD_NUMBER: _ClassVar[int]
    SUBJECT_ID_FIELD_NUMBER: _ClassVar[int]
    ACTOR_FIELD_NUMBER: _ClassVar[int]
    CAUSE_FIELD_NUMBER: _ClassVar[int]
    MESSAGE_FIELD_NUMBER: _ClassVar[int]
    DETAIL_FIELD_NUMBER: _ClassVar[int]
    seq: int
    at: _timestamp_pb2.Timestamp
    unit: str
    incarnation: int
    source: str
    severity: int
    type: str
    subject_kind: str
    subject_id: str
    actor: str
    cause: int
    message: str
    detail: bytes
    def __init__(self, seq: _Optional[int] = ..., at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., unit: _Optional[str] = ..., incarnation: _Optional[int] = ..., source: _Optional[str] = ..., severity: _Optional[int] = ..., type: _Optional[str] = ..., subject_kind: _Optional[str] = ..., subject_id: _Optional[str] = ..., actor: _Optional[str] = ..., cause: _Optional[int] = ..., message: _Optional[str] = ..., detail: _Optional[bytes] = ...) -> None: ...

class Followed(_message.Message):
    __slots__ = ("entry", "behind_at")
    ENTRY_FIELD_NUMBER: _ClassVar[int]
    BEHIND_AT_FIELD_NUMBER: _ClassVar[int]
    entry: LogEntry
    behind_at: int
    def __init__(self, entry: _Optional[_Union[LogEntry, _Mapping]] = ..., behind_at: _Optional[int] = ...) -> None: ...

class Subscribed(_message.Message):
    __slots__ = ("snapshot", "event", "overflow")
    SNAPSHOT_FIELD_NUMBER: _ClassVar[int]
    EVENT_FIELD_NUMBER: _ClassVar[int]
    OVERFLOW_FIELD_NUMBER: _ClassVar[int]
    snapshot: _records_pb2.Snapshot
    event: _events_pb2.Event
    overflow: _records_pb2.Snapshot
    def __init__(self, snapshot: _Optional[_Union[_records_pb2.Snapshot, _Mapping]] = ..., event: _Optional[_Union[_events_pb2.Event, _Mapping]] = ..., overflow: _Optional[_Union[_records_pb2.Snapshot, _Mapping]] = ...) -> None: ...
