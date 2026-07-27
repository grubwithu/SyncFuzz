"""M0 resource-identity keys frozen by PRD §4.4."""

from __future__ import annotations

from typing import ClassVar, Literal

from pydantic import BaseModel, ConfigDict

from .errors import sf_assert

ResourceClass = Literal[
    "file",
    "unix_socket",
    "process",
    "listen_port",
    "executable",
    "shm",
    "mq",
]


class ResourceId(BaseModel):
    """Overview: carry one resource identity and derive its frozen comparison key."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    cls: ResourceClass
    dev: int | None = None
    ino: int | None = None
    content_hash: str | None = None
    abs_path: str | None = None
    pid: int | None = None
    starttime: int | None = None
    exe_hash: str | None = None
    name: str | None = None

    _KEYS: ClassVar[dict[ResourceClass, tuple[str, ...]]] = {
        "file": ("dev", "ino", "content_hash"),
        "unix_socket": ("pid", "starttime", "exe_hash"),
        "process": ("pid", "starttime"),
        "listen_port": ("ino", "pid", "starttime"),
        "executable": ("abs_path", "dev", "ino", "content_hash"),
        "shm": ("name", "ino", "pid", "starttime"),
        "mq": ("name", "ino", "pid", "starttime"),
    }

    def key(self) -> tuple[object, ...]:
        """Overview: return the complete PRD-defined identity tuple for this resource."""
        fields = self._KEYS[self.cls]
        values = tuple(getattr(self, field) for field in fields)
        sf_assert(
            all(value is not None for value in values),
            f"incomplete ResourceId for cls={self.cls}: {fields} -> {values}",
        )
        return (self.cls, *values)

    def __eq__(self, other: object) -> bool:
        """Overview: compare only the PRD-defined identity key for matching classes."""
        if not isinstance(other, ResourceId):
            return NotImplemented
        return self.key() == other.key()

    def __hash__(self) -> int:
        """Overview: make equal resource identities hash identically for set and dict use."""
        return hash(self.key())
