import datetime

from google.protobuf import timestamp_pb2 as _timestamp_pb2
from yoke.administrative.v1 import codes_pb2 as _codes_pb2
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Event(_message.Message):
    __slots__ = ("seq", "type", "subject", "time", "actor", "severity", "occurrence", "cause", "detail")
    SEQ_FIELD_NUMBER: _ClassVar[int]
    TYPE_FIELD_NUMBER: _ClassVar[int]
    SUBJECT_FIELD_NUMBER: _ClassVar[int]
    TIME_FIELD_NUMBER: _ClassVar[int]
    ACTOR_FIELD_NUMBER: _ClassVar[int]
    SEVERITY_FIELD_NUMBER: _ClassVar[int]
    OCCURRENCE_FIELD_NUMBER: _ClassVar[int]
    CAUSE_FIELD_NUMBER: _ClassVar[int]
    DETAIL_FIELD_NUMBER: _ClassVar[int]
    seq: int
    type: str
    subject: _codes_pb2.Subject
    time: _timestamp_pb2.Timestamp
    actor: Actor
    severity: int
    occurrence: str
    cause: int
    detail: bytes
    def __init__(self, seq: _Optional[int] = ..., type: _Optional[str] = ..., subject: _Optional[_Union[_codes_pb2.Subject, _Mapping]] = ..., time: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., actor: _Optional[_Union[Actor, _Mapping]] = ..., severity: _Optional[int] = ..., occurrence: _Optional[str] = ..., cause: _Optional[int] = ..., detail: _Optional[bytes] = ...) -> None: ...

class Actor(_message.Message):
    __slots__ = ("person",)
    CLASS_FIELD_NUMBER: _ClassVar[int]
    PERSON_FIELD_NUMBER: _ClassVar[int]
    person: str
    def __init__(self, person: _Optional[str] = ..., **kwargs) -> None: ...

class Filter(_message.Message):
    __slots__ = ("subject_kind", "subject_identity", "floor", "type", "type_prefix", "occurrence", "occurrence_prefix")
    SUBJECT_KIND_FIELD_NUMBER: _ClassVar[int]
    SUBJECT_IDENTITY_FIELD_NUMBER: _ClassVar[int]
    FLOOR_FIELD_NUMBER: _ClassVar[int]
    TYPE_FIELD_NUMBER: _ClassVar[int]
    TYPE_PREFIX_FIELD_NUMBER: _ClassVar[int]
    OCCURRENCE_FIELD_NUMBER: _ClassVar[int]
    OCCURRENCE_PREFIX_FIELD_NUMBER: _ClassVar[int]
    subject_kind: str
    subject_identity: str
    floor: int
    type: str
    type_prefix: bool
    occurrence: str
    occurrence_prefix: bool
    def __init__(self, subject_kind: _Optional[str] = ..., subject_identity: _Optional[str] = ..., floor: _Optional[int] = ..., type: _Optional[str] = ..., type_prefix: _Optional[bool] = ..., occurrence: _Optional[str] = ..., occurrence_prefix: _Optional[bool] = ...) -> None: ...
