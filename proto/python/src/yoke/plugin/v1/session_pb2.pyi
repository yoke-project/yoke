from yoke.plugin.v1 import families_pb2 as _families_pb2
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Envelope(_message.Message):
    __slots__ = ("message_id", "session_id", "sent_at_unix_nano", "correlation_id", "session", "control", "ack", "query", "data", "health", "event", "error")
    MESSAGE_ID_FIELD_NUMBER: _ClassVar[int]
    SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    SENT_AT_UNIX_NANO_FIELD_NUMBER: _ClassVar[int]
    CORRELATION_ID_FIELD_NUMBER: _ClassVar[int]
    SESSION_FIELD_NUMBER: _ClassVar[int]
    CONTROL_FIELD_NUMBER: _ClassVar[int]
    ACK_FIELD_NUMBER: _ClassVar[int]
    QUERY_FIELD_NUMBER: _ClassVar[int]
    DATA_FIELD_NUMBER: _ClassVar[int]
    HEALTH_FIELD_NUMBER: _ClassVar[int]
    EVENT_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    message_id: str
    session_id: str
    sent_at_unix_nano: int
    correlation_id: str
    session: _families_pb2.SessionMessage
    control: _families_pb2.Control
    ack: _families_pb2.Ack
    query: _families_pb2.Query
    data: _families_pb2.Data
    health: _families_pb2.Health
    event: _families_pb2.Event
    error: _families_pb2.Error
    def __init__(self, message_id: _Optional[str] = ..., session_id: _Optional[str] = ..., sent_at_unix_nano: _Optional[int] = ..., correlation_id: _Optional[str] = ..., session: _Optional[_Union[_families_pb2.SessionMessage, _Mapping]] = ..., control: _Optional[_Union[_families_pb2.Control, _Mapping]] = ..., ack: _Optional[_Union[_families_pb2.Ack, _Mapping]] = ..., query: _Optional[_Union[_families_pb2.Query, _Mapping]] = ..., data: _Optional[_Union[_families_pb2.Data, _Mapping]] = ..., health: _Optional[_Union[_families_pb2.Health, _Mapping]] = ..., event: _Optional[_Union[_families_pb2.Event, _Mapping]] = ..., error: _Optional[_Union[_families_pb2.Error, _Mapping]] = ...) -> None: ...
