"""HTTP request handlers for the user management API."""
from typing import List, Tuple
from .models import User, CreateUserRequest, UpdateUserRequest, validate_email, parse_user_id


class UserNotFoundError(Exception):
    pass


class ValidationError(Exception):
    pass


async def get_user(user_id: str, service) -> dict:
    """Fetch a single user by ID.

    Args:
        user_id: The unique user identifier.
        service: UserService instance providing data access.

    Returns:
        Serialized user dict.

    Raises:
        UserNotFoundError: If no user exists with the given ID.
    """
    uid = parse_user_id(user_id)
    user = await service.find_by_id(uid)
    if user is None:
        raise UserNotFoundError(f"user {uid!r} not found")
    return user.to_dict()


async def create_user(data: CreateUserRequest, service) -> dict:
    """Create a new user account.

    Validates the email address and delegates creation to the service.
    Returns the newly created user's serialized representation.
    """
    if not validate_email(data.email):
        raise ValidationError(f"invalid email address: {data.email!r}")
    user = await service.create(data)
    return user.to_dict()


async def delete_user(user_id: str, service) -> None:
    """Permanently delete a user account by ID."""
    uid = parse_user_id(user_id)
    exists = await service.find_by_id(uid)
    if exists is None:
        raise UserNotFoundError(f"user {uid!r} not found")
    await service.delete(uid)


async def list_users(service, page: int = 1, page_size: int = 20) -> List[dict]:
    """Return a paginated list of all users."""
    offset, limit = validate_pagination(page, page_size)
    users = await service.list(offset=offset, limit=limit)
    return [u.to_dict() for u in users]


def validate_pagination(page: int, size: int) -> Tuple[int, int]:
    """Validate pagination parameters and return (offset, limit).

    Raises:
        ValidationError: If page < 1 or size is outside [1, 100].
    """
    if page < 1:
        raise ValidationError("page must be >= 1")
    if not (1 <= size <= 100):
        raise ValidationError("page_size must be between 1 and 100")
    return (page - 1) * size, size
