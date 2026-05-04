"""Pydantic / dataclass models for the API layer."""
import re
from dataclasses import dataclass, field
from typing import Optional
from enum import Enum


class Role(str, Enum):
    ADMIN = "admin"
    EDITOR = "editor"
    VIEWER = "viewer"
    GUEST = "guest"


class Permission(str, Enum):
    READ = "read"
    WRITE = "write"
    DELETE = "delete"
    MANAGE_USERS = "manage_users"
    VIEW_LOGS = "view_logs"


@dataclass
class User:
    """Core user model stored in the database."""
    id: str
    email: str
    role: Role = Role.VIEWER
    display_name: str = ""
    is_active: bool = True
    hashed_password: str = field(default="", repr=False)

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "email": self.email,
            "role": self.role.value,
            "display_name": self.display_name,
            "is_active": self.is_active,
        }


@dataclass
class CreateUserRequest:
    email: str
    password: str
    role: Role = Role.VIEWER
    display_name: str = ""


@dataclass
class UpdateUserRequest:
    display_name: Optional[str] = None
    role: Optional[Role] = None
    is_active: Optional[bool] = None


_EMAIL_RE = re.compile(r"^[a-zA-Z0-9_.+-]+@[a-zA-Z0-9-]+\.[a-zA-Z0-9-.]+$")


def validate_email(email: str) -> bool:
    """Return True if email matches a basic RFC-5322 pattern."""
    return bool(_EMAIL_RE.match(email.strip()))


def parse_user_id(raw: str) -> str:
    """Strip whitespace and validate that raw is a non-empty user ID string."""
    uid = raw.strip()
    if not uid:
        raise ValueError("user_id must not be empty")
    return uid
