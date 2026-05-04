"""JWT token utilities for authentication."""
import hmac
import hashlib
import json
import time
import base64


class TokenExpiredError(Exception):
    """Raised when a JWT token has passed its expiry time."""
    pass


class InvalidTokenError(Exception):
    """Raised when a JWT token is malformed or has an invalid signature."""
    pass


def encode_token(user_id: str, secret: str, expires_in: int = 3600) -> str:
    """Encode a JWT token for the given user_id.

    Args:
        user_id: The unique identifier of the authenticated user.
        secret: HMAC-SHA256 signing secret.
        expires_in: Token lifetime in seconds (default: 1 hour).

    Returns:
        A signed JWT token string (header.payload.signature).
    """
    header = {"alg": "HS256", "typ": "JWT"}
    payload = {
        "sub": user_id,
        "iat": int(time.time()),
        "exp": int(time.time()) + expires_in,
    }
    header_b64 = base64.urlsafe_b64encode(json.dumps(header).encode()).rstrip(b"=").decode()
    payload_b64 = base64.urlsafe_b64encode(json.dumps(payload).encode()).rstrip(b"=").decode()
    signing_input = f"{header_b64}.{payload_b64}"
    sig = hmac.new(secret.encode(), signing_input.encode(), hashlib.sha256).digest()
    sig_b64 = base64.urlsafe_b64encode(sig).rstrip(b"=").decode()
    return f"{signing_input}.{sig_b64}"


def decode_token(token: str, secret: str) -> dict:
    """Decode and verify a JWT token.

    Raises:
        InvalidTokenError: If the token structure or signature is invalid.
        TokenExpiredError: If the token has expired.

    Returns:
        The decoded payload as a dictionary.
    """
    parts = token.split(".")
    if len(parts) != 3:
        raise InvalidTokenError("token must have three parts")
    header_b64, payload_b64, sig_b64 = parts
    signing_input = f"{header_b64}.{payload_b64}"
    expected_sig = hmac.new(secret.encode(), signing_input.encode(), hashlib.sha256).digest()
    expected_b64 = base64.urlsafe_b64encode(expected_sig).rstrip(b"=").decode()
    if not hmac.compare_digest(sig_b64, expected_b64):
        raise InvalidTokenError("signature verification failed")
    padding = 4 - len(payload_b64) % 4
    payload = json.loads(base64.urlsafe_b64decode(payload_b64 + "=" * padding))
    if payload.get("exp", 0) < int(time.time()):
        raise TokenExpiredError("token has expired")
    return payload


def refresh_token(token: str, secret: str, extends_by: int = 3600) -> str:
    """Issue a new token with a fresh expiry from an existing (possibly expired) token.

    The original token's subject (user_id) is preserved; only the expiry is reset.
    """
    parts = token.split(".")
    if len(parts) != 3:
        raise InvalidTokenError("malformed token")
    padding = 4 - len(parts[1]) % 4
    payload = json.loads(base64.urlsafe_b64decode(parts[1] + "=" * padding))
    return encode_token(payload["sub"], secret, extends_by)
