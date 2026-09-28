from yoke.administrative.v1 import codes_pb2 as _codes_pb2
from yoke.administrative.v1 import events_pb2 as _events_pb2
from yoke.administrative.v1 import operations_pb2 as _operations_pb2
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class ClientFrame(_message.Message):
    __slots__ = ("call", "request", "cancel")
    CALL_FIELD_NUMBER: _ClassVar[int]
    REQUEST_FIELD_NUMBER: _ClassVar[int]
    CANCEL_FIELD_NUMBER: _ClassVar[int]
    call: str
    request: _operations_pb2.Request
    cancel: Cancel
    def __init__(self, call: _Optional[str] = ..., request: _Optional[_Union[_operations_pb2.Request, _Mapping]] = ..., cancel: _Optional[_Union[Cancel, _Mapping]] = ...) -> None: ...

class Cancel(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class CoreFrame(_message.Message):
    __slots__ = ("call", "opening", "answer", "event", "refusal", "completion")
    CALL_FIELD_NUMBER: _ClassVar[int]
    OPENING_FIELD_NUMBER: _ClassVar[int]
    ANSWER_FIELD_NUMBER: _ClassVar[int]
    EVENT_FIELD_NUMBER: _ClassVar[int]
    REFUSAL_FIELD_NUMBER: _ClassVar[int]
    COMPLETION_FIELD_NUMBER: _ClassVar[int]
    call: str
    opening: Opening
    answer: _operations_pb2.Response
    event: _events_pb2.Event
    refusal: _codes_pb2.Refusal
    completion: Completion
    def __init__(self, call: _Optional[str] = ..., opening: _Optional[_Union[Opening, _Mapping]] = ..., answer: _Optional[_Union[_operations_pb2.Response, _Mapping]] = ..., event: _Optional[_Union[_events_pb2.Event, _Mapping]] = ..., refusal: _Optional[_Union[_codes_pb2.Refusal, _Mapping]] = ..., completion: _Optional[_Union[Completion, _Mapping]] = ...) -> None: ...

class Opening(_message.Message):
    __slots__ = ("connection", "actor", "subscription", "version")
    CONNECTION_FIELD_NUMBER: _ClassVar[int]
    ACTOR_FIELD_NUMBER: _ClassVar[int]
    SUBSCRIPTION_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    connection: str
    actor: _events_pb2.Actor
    subscription: str
    version: int
    def __init__(self, connection: _Optional[str] = ..., actor: _Optional[_Union[_events_pb2.Actor, _Mapping]] = ..., subscription: _Optional[str] = ..., version: _Optional[int] = ...) -> None: ...

class Completion(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...
