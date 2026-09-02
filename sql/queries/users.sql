-- name: CreateUser :one
INSERT INTO users (id, created_at, updated_at, email, hashed_password)
VALUES (
	gen_random_uuid(),
	NOW(),
	NOW(),
	$1,
	$2
)
RETURNING *;

-- name: ResetTable :exec
DELETE FROM users;

-- name: GetUserByEmail :one
SELECT * from users
WHERE email = $1;
--
-- name: GetUserById :one
SELECT * from users
WHERE id = $1;

-- name: UpdateUser :one
UPDATE users
SET email = $1, hashed_password = $2, updated_at = NOW()
WHERE id = $3
RETURNING *;

-- name: UpdateRedStatus :exec
UPDATE users
SET is_chirpy_red = $1
WHERE id = $2;
