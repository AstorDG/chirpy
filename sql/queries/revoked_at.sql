-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (token, created_at, updated_at, user_id, expires_at, revoked_at)
VALUES (
	$1,
	NOW(),
	NOW(),
	$2,
	$3,
	NULL
)
RETURNING *;

-- name: GetRefreshToken :one
SELECT user_id, expires_at, revoked_at, token from refresh_tokens
where $1 = token;

-- name: RevokeToken :exec
UPDATE refresh_tokens
SET revoked_at = $1, updated_at = $1
WHERE token = $2;
