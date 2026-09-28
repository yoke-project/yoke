import datetime

from google.protobuf import duration_pb2 as _duration_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Stage(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    STAGE_UNSPECIFIED: _ClassVar[Stage]
    STAGE_STRUCTURAL: _ClassVar[Stage]
    STAGE_IDENTITY: _ClassVar[Stage]
    STAGE_ADMINISTRATIVE_STATE: _ClassVar[Stage]
    STAGE_AUTHENTICATION: _ClassVar[Stage]
    STAGE_COMPATIBILITY: _ClassVar[Stage]
    STAGE_DECLARATION_CONSISTENCY: _ClassVar[Stage]
    STAGE_AUTHORISATION: _ClassVar[Stage]
    STAGE_UNIT_CONFLICT: _ClassVar[Stage]
    STAGE_SESSION_PREPARATION: _ClassVar[Stage]
STAGE_UNSPECIFIED: Stage
STAGE_STRUCTURAL: Stage
STAGE_IDENTITY: Stage
STAGE_ADMINISTRATIVE_STATE: Stage
STAGE_AUTHENTICATION: Stage
STAGE_COMPATIBILITY: Stage
STAGE_DECLARATION_CONSISTENCY: Stage
STAGE_AUTHORISATION: Stage
STAGE_UNIT_CONFLICT: Stage
STAGE_SESSION_PREPARATION: Stage

class RegisterRequest(_message.Message):
    __slots__ = ("plugin", "unit", "token", "protocol", "artifact_version", "language", "sdk_line", "declared")
    PLUGIN_FIELD_NUMBER: _ClassVar[int]
    UNIT_FIELD_NUMBER: _ClassVar[int]
    TOKEN_FIELD_NUMBER: _ClassVar[int]
    PROTOCOL_FIELD_NUMBER: _ClassVar[int]
    ARTIFACT_VERSION_FIELD_NUMBER: _ClassVar[int]
    LANGUAGE_FIELD_NUMBER: _ClassVar[int]
    SDK_LINE_FIELD_NUMBER: _ClassVar[int]
    DECLARED_FIELD_NUMBER: _ClassVar[int]
    plugin: str
    unit: str
    token: str
    protocol: int
    artifact_version: str
    language: str
    sdk_line: str
    declared: Surface
    def __init__(self, plugin: _Optional[str] = ..., unit: _Optional[str] = ..., token: _Optional[str] = ..., protocol: _Optional[int] = ..., artifact_version: _Optional[str] = ..., language: _Optional[str] = ..., sdk_line: _Optional[str] = ..., declared: _Optional[_Union[Surface, _Mapping]] = ...) -> None: ...

class Surface(_message.Message):
    __slots__ = ("capabilities", "streams", "commands", "queries", "occurrences")
    CAPABILITIES_FIELD_NUMBER: _ClassVar[int]
    STREAMS_FIELD_NUMBER: _ClassVar[int]
    COMMANDS_FIELD_NUMBER: _ClassVar[int]
    QUERIES_FIELD_NUMBER: _ClassVar[int]
    OCCURRENCES_FIELD_NUMBER: _ClassVar[int]
    capabilities: _containers.RepeatedScalarFieldContainer[str]
    streams: _containers.RepeatedScalarFieldContainer[str]
    commands: _containers.RepeatedScalarFieldContainer[str]
    queries: _containers.RepeatedScalarFieldContainer[str]
    occurrences: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, capabilities: _Optional[_Iterable[str]] = ..., streams: _Optional[_Iterable[str]] = ..., commands: _Optional[_Iterable[str]] = ..., queries: _Optional[_Iterable[str]] = ..., occurrences: _Optional[_Iterable[str]] = ...) -> None: ...

class RegisterResponse(_message.Message):
    __slots__ = ("outcome", "stage", "code", "message", "session_id", "granted", "withheld", "heartbeat")
    class Outcome(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
        __slots__ = ()
        OUTCOME_UNSPECIFIED: _ClassVar[RegisterResponse.Outcome]
        OUTCOME_ACCEPTED: _ClassVar[RegisterResponse.Outcome]
        OUTCOME_ACCEPTED_WITH_RESTRICTIONS: _ClassVar[RegisterResponse.Outcome]
        OUTCOME_REFUSED: _ClassVar[RegisterResponse.Outcome]
    OUTCOME_UNSPECIFIED: RegisterResponse.Outcome
    OUTCOME_ACCEPTED: RegisterResponse.Outcome
    OUTCOME_ACCEPTED_WITH_RESTRICTIONS: RegisterResponse.Outcome
    OUTCOME_REFUSED: RegisterResponse.Outcome
    OUTCOME_FIELD_NUMBER: _ClassVar[int]
    STAGE_FIELD_NUMBER: _ClassVar[int]
    CODE_FIELD_NUMBER: _ClassVar[int]
    MESSAGE_FIELD_NUMBER: _ClassVar[int]
    SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    GRANTED_FIELD_NUMBER: _ClassVar[int]
    WITHHELD_FIELD_NUMBER: _ClassVar[int]
    HEARTBEAT_FIELD_NUMBER: _ClassVar[int]
    outcome: RegisterResponse.Outcome
    stage: Stage
    code: str
    message: str
    session_id: str
    granted: Surface
    withheld: Surface
    heartbeat: HeartbeatTerms
    def __init__(self, outcome: _Optional[_Union[RegisterResponse.Outcome, str]] = ..., stage: _Optional[_Union[Stage, str]] = ..., code: _Optional[str] = ..., message: _Optional[str] = ..., session_id: _Optional[str] = ..., granted: _Optional[_Union[Surface, _Mapping]] = ..., withheld: _Optional[_Union[Surface, _Mapping]] = ..., heartbeat: _Optional[_Union[HeartbeatTerms, _Mapping]] = ...) -> None: ...

class HeartbeatTerms(_message.Message):
    __slots__ = ("interval", "tolerance")
    INTERVAL_FIELD_NUMBER: _ClassVar[int]
    TOLERANCE_FIELD_NUMBER: _ClassVar[int]
    interval: _duration_pb2.Duration
    tolerance: int
    def __init__(self, interval: _Optional[_Union[datetime.timedelta, _duration_pb2.Duration, _Mapping]] = ..., tolerance: _Optional[int] = ...) -> None: ...
