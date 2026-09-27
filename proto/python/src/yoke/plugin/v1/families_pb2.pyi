from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class SessionMessage(_message.Message):
    __slots__ = ("open", "close", "revoked")
    class Open(_message.Message):
        __slots__ = ()
        def __init__(self) -> None: ...
    class Close(_message.Message):
        __slots__ = ()
        def __init__(self) -> None: ...
    class Revoked(_message.Message):
        __slots__ = ("cause", "line")
        class Cause(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
            __slots__ = ()
            CAUSE_UNSPECIFIED: _ClassVar[SessionMessage.Revoked.Cause]
            CAUSE_LIVENESS_LOST: _ClassVar[SessionMessage.Revoked.Cause]
            CAUSE_PLUGIN_DISABLED: _ClassVar[SessionMessage.Revoked.Cause]
            CAUSE_SCOPE_EXCEEDED: _ClassVar[SessionMessage.Revoked.Cause]
            CAUSE_PROTOCOL_FAILURE: _ClassVar[SessionMessage.Revoked.Cause]
        CAUSE_UNSPECIFIED: SessionMessage.Revoked.Cause
        CAUSE_LIVENESS_LOST: SessionMessage.Revoked.Cause
        CAUSE_PLUGIN_DISABLED: SessionMessage.Revoked.Cause
        CAUSE_SCOPE_EXCEEDED: SessionMessage.Revoked.Cause
        CAUSE_PROTOCOL_FAILURE: SessionMessage.Revoked.Cause
        CAUSE_FIELD_NUMBER: _ClassVar[int]
        LINE_FIELD_NUMBER: _ClassVar[int]
        cause: SessionMessage.Revoked.Cause
        line: str
        def __init__(self, cause: _Optional[_Union[SessionMessage.Revoked.Cause, str]] = ..., line: _Optional[str] = ...) -> None: ...
    OPEN_FIELD_NUMBER: _ClassVar[int]
    CLOSE_FIELD_NUMBER: _ClassVar[int]
    REVOKED_FIELD_NUMBER: _ClassVar[int]
    open: SessionMessage.Open
    close: SessionMessage.Close
    revoked: SessionMessage.Revoked
    def __init__(self, open: _Optional[_Union[SessionMessage.Open, _Mapping]] = ..., close: _Optional[_Union[SessionMessage.Close, _Mapping]] = ..., revoked: _Optional[_Union[SessionMessage.Revoked, _Mapping]] = ...) -> None: ...

class Control(_message.Message):
    __slots__ = ("command", "activate", "stop")
    class Command(_message.Message):
        __slots__ = ("type", "payload")
        TYPE_FIELD_NUMBER: _ClassVar[int]
        PAYLOAD_FIELD_NUMBER: _ClassVar[int]
        type: str
        payload: bytes
        def __init__(self, type: _Optional[str] = ..., payload: _Optional[bytes] = ...) -> None: ...
    class Activate(_message.Message):
        __slots__ = ("stream", "transport", "address")
        class Transport(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
            __slots__ = ()
            TRANSPORT_UNSPECIFIED: _ClassVar[Control.Activate.Transport]
            TRANSPORT_ORDERED: _ClassVar[Control.Activate.Transport]
            TRANSPORT_FRAMED: _ClassVar[Control.Activate.Transport]
            TRANSPORT_SHARED_OBJECT: _ClassVar[Control.Activate.Transport]
        TRANSPORT_UNSPECIFIED: Control.Activate.Transport
        TRANSPORT_ORDERED: Control.Activate.Transport
        TRANSPORT_FRAMED: Control.Activate.Transport
        TRANSPORT_SHARED_OBJECT: Control.Activate.Transport
        STREAM_FIELD_NUMBER: _ClassVar[int]
        TRANSPORT_FIELD_NUMBER: _ClassVar[int]
        ADDRESS_FIELD_NUMBER: _ClassVar[int]
        stream: str
        transport: Control.Activate.Transport
        address: str
        def __init__(self, stream: _Optional[str] = ..., transport: _Optional[_Union[Control.Activate.Transport, str]] = ..., address: _Optional[str] = ...) -> None: ...
    class Stop(_message.Message):
        __slots__ = ("stream",)
        STREAM_FIELD_NUMBER: _ClassVar[int]
        stream: str
        def __init__(self, stream: _Optional[str] = ...) -> None: ...
    COMMAND_FIELD_NUMBER: _ClassVar[int]
    ACTIVATE_FIELD_NUMBER: _ClassVar[int]
    STOP_FIELD_NUMBER: _ClassVar[int]
    command: Control.Command
    activate: Control.Activate
    stop: Control.Stop
    def __init__(self, command: _Optional[_Union[Control.Command, _Mapping]] = ..., activate: _Optional[_Union[Control.Activate, _Mapping]] = ..., stop: _Optional[_Union[Control.Stop, _Mapping]] = ...) -> None: ...

class Ack(_message.Message):
    __slots__ = ("outcome", "line")
    class Outcome(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
        __slots__ = ()
        OUTCOME_UNSPECIFIED: _ClassVar[Ack.Outcome]
        OUTCOME_ACCEPTED: _ClassVar[Ack.Outcome]
        OUTCOME_DONE: _ClassVar[Ack.Outcome]
        OUTCOME_FAILED: _ClassVar[Ack.Outcome]
    OUTCOME_UNSPECIFIED: Ack.Outcome
    OUTCOME_ACCEPTED: Ack.Outcome
    OUTCOME_DONE: Ack.Outcome
    OUTCOME_FAILED: Ack.Outcome
    OUTCOME_FIELD_NUMBER: _ClassVar[int]
    LINE_FIELD_NUMBER: _ClassVar[int]
    outcome: Ack.Outcome
    line: str
    def __init__(self, outcome: _Optional[_Union[Ack.Outcome, str]] = ..., line: _Optional[str] = ...) -> None: ...

class Query(_message.Message):
    __slots__ = ("question", "answer")
    class Question(_message.Message):
        __slots__ = ("type", "payload")
        TYPE_FIELD_NUMBER: _ClassVar[int]
        PAYLOAD_FIELD_NUMBER: _ClassVar[int]
        type: str
        payload: bytes
        def __init__(self, type: _Optional[str] = ..., payload: _Optional[bytes] = ...) -> None: ...
    class Answer(_message.Message):
        __slots__ = ("payload",)
        PAYLOAD_FIELD_NUMBER: _ClassVar[int]
        payload: bytes
        def __init__(self, payload: _Optional[bytes] = ...) -> None: ...
    QUESTION_FIELD_NUMBER: _ClassVar[int]
    ANSWER_FIELD_NUMBER: _ClassVar[int]
    question: Query.Question
    answer: Query.Answer
    def __init__(self, question: _Optional[_Union[Query.Question, _Mapping]] = ..., answer: _Optional[_Union[Query.Answer, _Mapping]] = ...) -> None: ...

class Data(_message.Message):
    __slots__ = ("sequence", "payload")
    SEQUENCE_FIELD_NUMBER: _ClassVar[int]
    PAYLOAD_FIELD_NUMBER: _ClassVar[int]
    sequence: int
    payload: bytes
    def __init__(self, sequence: _Optional[int] = ..., payload: _Optional[bytes] = ...) -> None: ...

class Health(_message.Message):
    __slots__ = ("grade", "line")
    GRADE_FIELD_NUMBER: _ClassVar[int]
    LINE_FIELD_NUMBER: _ClassVar[int]
    grade: int
    line: str
    def __init__(self, grade: _Optional[int] = ..., line: _Optional[str] = ...) -> None: ...

class Event(_message.Message):
    __slots__ = ("occurrence", "severity", "line", "detail")
    OCCURRENCE_FIELD_NUMBER: _ClassVar[int]
    SEVERITY_FIELD_NUMBER: _ClassVar[int]
    LINE_FIELD_NUMBER: _ClassVar[int]
    DETAIL_FIELD_NUMBER: _ClassVar[int]
    occurrence: str
    severity: int
    line: str
    detail: bytes
    def __init__(self, occurrence: _Optional[str] = ..., severity: _Optional[int] = ..., line: _Optional[str] = ..., detail: _Optional[bytes] = ...) -> None: ...

class Error(_message.Message):
    __slots__ = ("code", "message", "divergent", "withheld")
    class Divergence(_message.Message):
        __slots__ = ("items",)
        ITEMS_FIELD_NUMBER: _ClassVar[int]
        items: _containers.RepeatedScalarFieldContainer[str]
        def __init__(self, items: _Optional[_Iterable[str]] = ...) -> None: ...
    class Withheld(_message.Message):
        __slots__ = ("item",)
        ITEM_FIELD_NUMBER: _ClassVar[int]
        item: str
        def __init__(self, item: _Optional[str] = ...) -> None: ...
    CODE_FIELD_NUMBER: _ClassVar[int]
    MESSAGE_FIELD_NUMBER: _ClassVar[int]
    DIVERGENT_FIELD_NUMBER: _ClassVar[int]
    WITHHELD_FIELD_NUMBER: _ClassVar[int]
    code: str
    message: str
    divergent: Error.Divergence
    withheld: Error.Withheld
    def __init__(self, code: _Optional[str] = ..., message: _Optional[str] = ..., divergent: _Optional[_Union[Error.Divergence, _Mapping]] = ..., withheld: _Optional[_Union[Error.Withheld, _Mapping]] = ...) -> None: ...
