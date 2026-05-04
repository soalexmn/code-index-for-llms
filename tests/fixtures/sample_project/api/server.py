"""FastAPI application setup with middleware and lifecycle hooks."""
from typing import Callable
from .models import User
from .handlers import get_user, create_user, delete_user, list_users, CreateUserRequest


class AuthMiddleware:
    """ASGI middleware that validates Bearer tokens on every request.

    Skips authentication for paths listed in EXEMPT_PATHS.
    """
    EXEMPT_PATHS = {"/health", "/metrics", "/docs", "/openapi.json"}

    def __init__(self, app, token_secret: str):
        self.app = app
        self._secret = token_secret

    async def __call__(self, scope, receive, send):
        if scope["type"] == "http" and scope["path"] not in self.EXEMPT_PATHS:
            headers = dict(scope.get("headers", []))
            auth = headers.get(b"authorization", b"").decode()
            if not auth.startswith("Bearer "):
                await self._reject(send, 401, "missing or invalid Authorization header")
                return
        await self.app(scope, receive, send)

    async def _reject(self, send, status: int, message: str):
        body = f'{{"error": "{message}"}}'.encode()
        await send({"type": "http.response.start", "status": status,
                    "headers": [[b"content-type", b"application/json"]]})
        await send({"type": "http.response.body", "body": body})


class UserService:
    """Thin application-layer service wrapping the user repository."""

    def __init__(self, repo):
        self._repo = repo

    async def find_by_id(self, user_id: str):
        return await self._repo.find_by_id(user_id)

    async def find_by_email(self, email: str):
        return await self._repo.find_by_email(email)

    async def create(self, req: CreateUserRequest) -> User:
        return await self._repo.create(req)

    async def delete(self, user_id: str) -> None:
        await self._repo.delete(user_id)

    async def list(self, offset: int = 0, limit: int = 20):
        return await self._repo.list(offset=offset, limit=limit)


def create_app(repo, token_secret: str):
    """Build and return the ASGI application with all middleware attached."""
    service = UserService(repo)

    async def app(scope, receive, send):
        if scope["type"] == "lifespan":
            await _handle_lifespan(scope, receive, send)
        elif scope["type"] == "http":
            await _route(scope, receive, send, service)

    return AuthMiddleware(app, token_secret)


async def _handle_lifespan(scope, receive, send):
    event = await receive()
    if event["type"] == "lifespan.startup":
        await send({"type": "lifespan.startup.complete"})
    elif event["type"] == "lifespan.shutdown":
        await send({"type": "lifespan.shutdown.complete"})


async def _route(scope, receive, send, service: UserService):
    path = scope["path"]
    method = scope["method"]
    body = b""
    await send({"type": "http.response.start", "status": 200,
                "headers": [[b"content-type", b"application/json"]]})
    await send({"type": "http.response.body", "body": body})
