from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from typing import ClassVar as _ClassVar

DESCRIPTOR: _descriptor.FileDescriptor

class Contract(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    CONTRACT_UNSPECIFIED: _ClassVar[Contract]
    CONTRACT_VERSION: _ClassVar[Contract]
CONTRACT_UNSPECIFIED: Contract
CONTRACT_VERSION: Contract
