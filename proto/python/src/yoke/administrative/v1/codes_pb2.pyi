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
    CODE_SUBJECT_UNKNOWN: _ClassVar[Code]
    CODE_SUBJECT_WRONG_KIND: _ClassVar[Code]
    CODE_CAPABILITY_UNDECLARED: _ClassVar[Code]
    CODE_STREAM_UNDECLARED: _ClassVar[Code]
    CODE_SCOPE_WITHHELD: _ClassVar[Code]
    CODE_UNIT_NOT_RUNNING: _ClassVar[Code]
    CODE_UNIT_NO_SESSION: _ClassVar[Code]
    CODE_UNIT_UNANSWERED: _ClassVar[Code]
    CODE_RETENTION_INVALID: _ClassVar[Code]
    CODE_INSTANCE_STOPPING: _ClassVar[Code]
    CODE_BACKEND_UNAVAILABLE: _ClassVar[Code]
    CODE_STORE_UNAVAILABLE: _ClassVar[Code]
CODE_UNSPECIFIED: Code
CODE_OPERATION_MALFORMED: Code
CODE_OPERATION_UNKNOWN: Code
CODE_COMPAT_UNSUPPORTED: Code
CODE_SUBJECT_UNKNOWN: Code
CODE_SUBJECT_WRONG_KIND: Code
CODE_CAPABILITY_UNDECLARED: Code
CODE_STREAM_UNDECLARED: Code
CODE_SCOPE_WITHHELD: Code
CODE_UNIT_NOT_RUNNING: Code
CODE_UNIT_NO_SESSION: Code
CODE_UNIT_UNANSWERED: Code
CODE_RETENTION_INVALID: Code
CODE_INSTANCE_STOPPING: Code
CODE_BACKEND_UNAVAILABLE: Code
CODE_STORE_UNAVAILABLE: Code
CODE_FIELD_NUMBER: _ClassVar[int]
code: _descriptor.FieldDescriptor

class Refusal(_message.Message):
    __slots__ = ("code", "message", "subject", "item")
    CODE_FIELD_NUMBER: _ClassVar[int]
    MESSAGE_FIELD_NUMBER: _ClassVar[int]
    SUBJECT_FIELD_NUMBER: _ClassVar[int]
    ITEM_FIELD_NUMBER: _ClassVar[int]
    code: str
    message: str
    subject: Subject
    item: str
    def __init__(self, code: _Optional[str] = ..., message: _Optional[str] = ..., subject: _Optional[_Union[Subject, _Mapping]] = ..., item: _Optional[str] = ...) -> None: ...

class Subject(_message.Message):
    __slots__ = ("kind", "identity", "incarnation")
    KIND_FIELD_NUMBER: _ClassVar[int]
    IDENTITY_FIELD_NUMBER: _ClassVar[int]
    INCARNATION_FIELD_NUMBER: _ClassVar[int]
    kind: str
    identity: str
    incarnation: int
    def __init__(self, kind: _Optional[str] = ..., identity: _Optional[str] = ..., incarnation: _Optional[int] = ...) -> None: ...
