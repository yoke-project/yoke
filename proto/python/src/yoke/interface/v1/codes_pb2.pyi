from google.protobuf import descriptor_pb2 as _descriptor_pb2
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Code(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    CODE_UNSPECIFIED: _ClassVar[Code]
    CODE_OPERATION_MALFORMED: _ClassVar[Code]
    CODE_OPERATION_UNKNOWN: _ClassVar[Code]
    CODE_COMPAT_UNSUPPORTED: _ClassVar[Code]
    CODE_AUTH_REQUIRED: _ClassVar[Code]
    CODE_AUTH_INVALID: _ClassVar[Code]
    CODE_CHANNEL_IN_USE: _ClassVar[Code]
    CODE_CHANNEL_NOT_ATTACHED: _ClassVar[Code]
    CODE_CHANNEL_SUSPENDED: _ClassVar[Code]
    CODE_CHANNEL_NOT_LOCAL: _ClassVar[Code]
    CODE_SUBJECT_UNKNOWN: _ClassVar[Code]
    CODE_SCOPE_UNDECLARED: _ClassVar[Code]
    CODE_SCOPE_WITHHELD: _ClassVar[Code]
    CODE_UNIT_NOT_RUNNING: _ClassVar[Code]
    CODE_UNIT_NO_SESSION: _ClassVar[Code]
    CODE_UNIT_UNANSWERED: _ClassVar[Code]
    CODE_INSTANCE_STOPPING: _ClassVar[Code]
    CODE_UNIT_FAILED: _ClassVar[Code]
CODE_UNSPECIFIED: Code
CODE_OPERATION_MALFORMED: Code
CODE_OPERATION_UNKNOWN: Code
CODE_COMPAT_UNSUPPORTED: Code
CODE_AUTH_REQUIRED: Code
CODE_AUTH_INVALID: Code
CODE_CHANNEL_IN_USE: Code
CODE_CHANNEL_NOT_ATTACHED: Code
CODE_CHANNEL_SUSPENDED: Code
CODE_CHANNEL_NOT_LOCAL: Code
CODE_SUBJECT_UNKNOWN: Code
CODE_SCOPE_UNDECLARED: Code
CODE_SCOPE_WITHHELD: Code
CODE_UNIT_NOT_RUNNING: Code
CODE_UNIT_NO_SESSION: Code
CODE_UNIT_UNANSWERED: Code
CODE_INSTANCE_STOPPING: Code
CODE_UNIT_FAILED: Code
CODE_FIELD_NUMBER: _ClassVar[int]
code: _descriptor.FieldDescriptor

class Refusal(_message.Message):
    __slots__ = ("code", "message", "subject", "item", "suspension")
    CODE_FIELD_NUMBER: _ClassVar[int]
    MESSAGE_FIELD_NUMBER: _ClassVar[int]
    SUBJECT_FIELD_NUMBER: _ClassVar[int]
    ITEM_FIELD_NUMBER: _ClassVar[int]
    SUSPENSION_FIELD_NUMBER: _ClassVar[int]
    code: str
    message: str
    subject: Subject
    item: str
    suspension: Suspension
    def __init__(self, code: _Optional[str] = ..., message: _Optional[str] = ..., subject: _Optional[_Union[Subject, _Mapping]] = ..., item: _Optional[str] = ..., suspension: _Optional[_Union[Suspension, _Mapping]] = ...) -> None: ...

class Subject(_message.Message):
    __slots__ = ("kind", "identity", "incarnation")
    KIND_FIELD_NUMBER: _ClassVar[int]
    IDENTITY_FIELD_NUMBER: _ClassVar[int]
    INCARNATION_FIELD_NUMBER: _ClassVar[int]
    kind: str
    identity: str
    incarnation: int
    def __init__(self, kind: _Optional[str] = ..., identity: _Optional[str] = ..., incarnation: _Optional[int] = ...) -> None: ...

class Suspension(_message.Message):
    __slots__ = ("grade", "by")
    GRADE_FIELD_NUMBER: _ClassVar[int]
    BY_FIELD_NUMBER: _ClassVar[int]
    grade: str
    by: str
    def __init__(self, grade: _Optional[str] = ..., by: _Optional[str] = ...) -> None: ...
