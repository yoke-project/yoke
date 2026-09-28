import datetime

from google.protobuf import timestamp_pb2 as _timestamp_pb2
from yoke.administrative.v1 import events_pb2 as _events_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Record(_message.Message):
    __slots__ = ("instance", "unit", "plugin", "channel", "document", "connection")
    INSTANCE_FIELD_NUMBER: _ClassVar[int]
    UNIT_FIELD_NUMBER: _ClassVar[int]
    PLUGIN_FIELD_NUMBER: _ClassVar[int]
    CHANNEL_FIELD_NUMBER: _ClassVar[int]
    DOCUMENT_FIELD_NUMBER: _ClassVar[int]
    CONNECTION_FIELD_NUMBER: _ClassVar[int]
    instance: InstanceRecord
    unit: UnitRecord
    plugin: PluginRecord
    channel: ChannelRecord
    document: DocumentRecord
    connection: ConnectionRecord
    def __init__(self, instance: _Optional[_Union[InstanceRecord, _Mapping]] = ..., unit: _Optional[_Union[UnitRecord, _Mapping]] = ..., plugin: _Optional[_Union[PluginRecord, _Mapping]] = ..., channel: _Optional[_Union[ChannelRecord, _Mapping]] = ..., document: _Optional[_Union[DocumentRecord, _Mapping]] = ..., connection: _Optional[_Union[ConnectionRecord, _Mapping]] = ...) -> None: ...

class InstanceRecord(_message.Message):
    __slots__ = ("identity", "form", "ready", "stopping", "since", "version", "stamp", "parameters", "description", "description_digest", "weaker")
    class ParametersEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: str
        def __init__(self, key: _Optional[str] = ..., value: _Optional[str] = ...) -> None: ...
    IDENTITY_FIELD_NUMBER: _ClassVar[int]
    FORM_FIELD_NUMBER: _ClassVar[int]
    READY_FIELD_NUMBER: _ClassVar[int]
    STOPPING_FIELD_NUMBER: _ClassVar[int]
    SINCE_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    STAMP_FIELD_NUMBER: _ClassVar[int]
    PARAMETERS_FIELD_NUMBER: _ClassVar[int]
    DESCRIPTION_FIELD_NUMBER: _ClassVar[int]
    DESCRIPTION_DIGEST_FIELD_NUMBER: _ClassVar[int]
    WEAKER_FIELD_NUMBER: _ClassVar[int]
    identity: str
    form: str
    ready: bool
    stopping: bool
    since: _timestamp_pb2.Timestamp
    version: str
    stamp: str
    parameters: _containers.ScalarMap[str, str]
    description: str
    description_digest: str
    weaker: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, identity: _Optional[str] = ..., form: _Optional[str] = ..., ready: _Optional[bool] = ..., stopping: _Optional[bool] = ..., since: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., version: _Optional[str] = ..., stamp: _Optional[str] = ..., parameters: _Optional[_Mapping[str, str]] = ..., description: _Optional[str] = ..., description_digest: _Optional[str] = ..., weaker: _Optional[_Iterable[str]] = ...) -> None: ...

class UnitRecord(_message.Message):
    __slots__ = ("declared", "observed", "plugin")
    class Declared(_message.Message):
        __slots__ = ("identity", "kind", "backend", "plugin")
        IDENTITY_FIELD_NUMBER: _ClassVar[int]
        KIND_FIELD_NUMBER: _ClassVar[int]
        BACKEND_FIELD_NUMBER: _ClassVar[int]
        PLUGIN_FIELD_NUMBER: _ClassVar[int]
        identity: str
        kind: str
        backend: str
        plugin: str
        def __init__(self, identity: _Optional[str] = ..., kind: _Optional[str] = ..., backend: _Optional[str] = ..., plugin: _Optional[str] = ...) -> None: ...
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
    DECLARED_FIELD_NUMBER: _ClassVar[int]
    OBSERVED_FIELD_NUMBER: _ClassVar[int]
    PLUGIN_FIELD_NUMBER: _ClassVar[int]
    declared: UnitRecord.Declared
    observed: UnitRecord.Observed
    plugin: PluginRecord
    def __init__(self, declared: _Optional[_Union[UnitRecord.Declared, _Mapping]] = ..., observed: _Optional[_Union[UnitRecord.Observed, _Mapping]] = ..., plugin: _Optional[_Union[PluginRecord, _Mapping]] = ...) -> None: ...

class Condition(_message.Message):
    __slots__ = ("grade", "line", "since")
    GRADE_FIELD_NUMBER: _ClassVar[int]
    LINE_FIELD_NUMBER: _ClassVar[int]
    SINCE_FIELD_NUMBER: _ClassVar[int]
    grade: int
    line: str
    since: _timestamp_pb2.Timestamp
    def __init__(self, grade: _Optional[int] = ..., line: _Optional[str] = ..., since: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class PluginRecord(_message.Message):
    __slots__ = ("declared", "authorized", "observed")
    class Declared(_message.Message):
        __slots__ = ("identity", "protocol", "manifest_digest", "version", "language", "sdk_line")
        IDENTITY_FIELD_NUMBER: _ClassVar[int]
        PROTOCOL_FIELD_NUMBER: _ClassVar[int]
        MANIFEST_DIGEST_FIELD_NUMBER: _ClassVar[int]
        VERSION_FIELD_NUMBER: _ClassVar[int]
        LANGUAGE_FIELD_NUMBER: _ClassVar[int]
        SDK_LINE_FIELD_NUMBER: _ClassVar[int]
        identity: str
        protocol: int
        manifest_digest: str
        version: str
        language: str
        sdk_line: str
        def __init__(self, identity: _Optional[str] = ..., protocol: _Optional[int] = ..., manifest_digest: _Optional[str] = ..., version: _Optional[str] = ..., language: _Optional[str] = ..., sdk_line: _Optional[str] = ...) -> None: ...
    class Authorized(_message.Message):
        __slots__ = ("enabled", "granted", "credential_mode")
        ENABLED_FIELD_NUMBER: _ClassVar[int]
        GRANTED_FIELD_NUMBER: _ClassVar[int]
        CREDENTIAL_MODE_FIELD_NUMBER: _ClassVar[int]
        enabled: bool
        granted: _containers.RepeatedScalarFieldContainer[str]
        credential_mode: str
        def __init__(self, enabled: _Optional[bool] = ..., granted: _Optional[_Iterable[str]] = ..., credential_mode: _Optional[str] = ...) -> None: ...
    class Observed(_message.Message):
        __slots__ = ("manifest_present", "composed", "running")
        MANIFEST_PRESENT_FIELD_NUMBER: _ClassVar[int]
        COMPOSED_FIELD_NUMBER: _ClassVar[int]
        RUNNING_FIELD_NUMBER: _ClassVar[int]
        manifest_present: bool
        composed: bool
        running: _containers.RepeatedScalarFieldContainer[str]
        def __init__(self, manifest_present: _Optional[bool] = ..., composed: _Optional[bool] = ..., running: _Optional[_Iterable[str]] = ...) -> None: ...
    DECLARED_FIELD_NUMBER: _ClassVar[int]
    AUTHORIZED_FIELD_NUMBER: _ClassVar[int]
    OBSERVED_FIELD_NUMBER: _ClassVar[int]
    declared: PluginRecord.Declared
    authorized: PluginRecord.Authorized
    observed: PluginRecord.Observed
    def __init__(self, declared: _Optional[_Union[PluginRecord.Declared, _Mapping]] = ..., authorized: _Optional[_Union[PluginRecord.Authorized, _Mapping]] = ..., observed: _Optional[_Union[PluginRecord.Observed, _Mapping]] = ...) -> None: ...

class ChannelRecord(_message.Message):
    __slots__ = ("declared", "observed")
    class Declared(_message.Message):
        __slots__ = ("name", "projection", "address_class")
        NAME_FIELD_NUMBER: _ClassVar[int]
        PROJECTION_FIELD_NUMBER: _ClassVar[int]
        ADDRESS_CLASS_FIELD_NUMBER: _ClassVar[int]
        name: str
        projection: str
        address_class: str
        def __init__(self, name: _Optional[str] = ..., projection: _Optional[str] = ..., address_class: _Optional[str] = ...) -> None: ...
    class Observed(_message.Message):
        __slots__ = ("attached", "client", "suspended", "reason", "retains")
        ATTACHED_FIELD_NUMBER: _ClassVar[int]
        CLIENT_FIELD_NUMBER: _ClassVar[int]
        SUSPENDED_FIELD_NUMBER: _ClassVar[int]
        REASON_FIELD_NUMBER: _ClassVar[int]
        RETAINS_FIELD_NUMBER: _ClassVar[int]
        attached: bool
        client: str
        suspended: bool
        reason: str
        retains: str
        def __init__(self, attached: _Optional[bool] = ..., client: _Optional[str] = ..., suspended: _Optional[bool] = ..., reason: _Optional[str] = ..., retains: _Optional[str] = ...) -> None: ...
    DECLARED_FIELD_NUMBER: _ClassVar[int]
    OBSERVED_FIELD_NUMBER: _ClassVar[int]
    declared: ChannelRecord.Declared
    observed: ChannelRecord.Observed
    def __init__(self, declared: _Optional[_Union[ChannelRecord.Declared, _Mapping]] = ..., observed: _Optional[_Union[ChannelRecord.Observed, _Mapping]] = ...) -> None: ...

class DocumentRecord(_message.Message):
    __slots__ = ("path", "resolved", "digest")
    PATH_FIELD_NUMBER: _ClassVar[int]
    RESOLVED_FIELD_NUMBER: _ClassVar[int]
    DIGEST_FIELD_NUMBER: _ClassVar[int]
    path: str
    resolved: str
    digest: str
    def __init__(self, path: _Optional[str] = ..., resolved: _Optional[str] = ..., digest: _Optional[str] = ...) -> None: ...

class ConnectionRecord(_message.Message):
    __slots__ = ("identity", "projection", "actor", "opened", "subscriptions")
    IDENTITY_FIELD_NUMBER: _ClassVar[int]
    PROJECTION_FIELD_NUMBER: _ClassVar[int]
    ACTOR_FIELD_NUMBER: _ClassVar[int]
    OPENED_FIELD_NUMBER: _ClassVar[int]
    SUBSCRIPTIONS_FIELD_NUMBER: _ClassVar[int]
    identity: str
    projection: str
    actor: _events_pb2.Actor
    opened: _timestamp_pb2.Timestamp
    subscriptions: _containers.RepeatedCompositeFieldContainer[_events_pb2.Filter]
    def __init__(self, identity: _Optional[str] = ..., projection: _Optional[str] = ..., actor: _Optional[_Union[_events_pb2.Actor, _Mapping]] = ..., opened: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., subscriptions: _Optional[_Iterable[_Union[_events_pb2.Filter, _Mapping]]] = ...) -> None: ...

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
