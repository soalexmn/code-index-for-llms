"""Role-based access control helpers."""
from typing import List

ROLE_MAP: dict = {
    "admin": ["read", "write", "delete", "manage_users", "view_logs"],
    "editor": ["read", "write"],
    "viewer": ["read"],
    "guest": [],
}


class PermissionChecker:
    """Evaluates whether a user role satisfies a required permission."""

    def __init__(self, role_map: dict = None):
        self._role_map = role_map or ROLE_MAP

    def check(self, user_role: str, required_permission: str) -> bool:
        """Return True if user_role has required_permission."""
        return required_permission in self._role_map.get(user_role, [])

    def get_all(self, user_role: str) -> List[str]:
        """Return all permissions granted to user_role."""
        return list(self._role_map.get(user_role, []))

    def add_permission(self, role: str, permission: str) -> None:
        """Dynamically grant a permission to a role."""
        self._role_map.setdefault(role, [])
        if permission not in self._role_map[role]:
            self._role_map[role].append(permission)


def has_permission(user_role: str, required: str) -> bool:
    """Module-level shortcut: check if user_role has the required permission.

    Uses the default ROLE_MAP. For custom role maps use PermissionChecker.
    """
    return required in ROLE_MAP.get(user_role, [])


def get_permissions(role: str) -> List[str]:
    """Return all permissions for a role from the default ROLE_MAP."""
    return list(ROLE_MAP.get(role, []))


def require_permission(user_role: str, required: str) -> None:
    """Raise PermissionError if user_role lacks the required permission."""
    if not has_permission(user_role, required):
        raise PermissionError(f"role '{user_role}' lacks permission '{required}'")
