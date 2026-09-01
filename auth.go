package main

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func hash_password(password string) (string, error) {
	hashed_password, err := argon2id.CreateHash(password, argon2id.DefaultParams)
	if err != nil {
		return "", err
	}
	return hashed_password, nil
}

func check_password_hash(password string, hash string) (bool, error) {
	match, err := argon2id.ComparePasswordAndHash(password, hash)
	if err != nil {
		return false, err
	}
	return match, nil
}

func make_jwt(userID uuid.UUID, token_secret string, expires_in time.Duration) (string, error) {
	byte_token := []byte(token_secret)
	new_token := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		jwt.RegisteredClaims{
			Issuer:    "chirpy-access",
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expires_in)),
			Subject:   userID.String(),
		})
	return new_token.SignedString(byte_token)
}

func validate_jwt(token_string string, token_secret string) (uuid.UUID, error) {
	claims := jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(
		token_string,
		&claims,
		func(t *jwt.Token) (any, error) {
			return []byte(token_secret), nil
		})
	if err != nil {
		log.Println(err)
		return uuid.Nil, err
	}
	string_id, err := token.Claims.GetSubject()
	if err != nil {
		return uuid.Nil, err
	}

	issuer, err := token.Claims.GetIssuer()
	if err != nil {
		return uuid.Nil, err
	}
	if issuer != "chirpy-access" {
		return uuid.Nil, errors.New("Invalid issuer")
	}

	unstringed_uuid, err := uuid.Parse(string_id)
	if err != nil {
		return uuid.Nil, err
	}

	return unstringed_uuid, nil
}

func get_bearer_token(headers http.Header) (string, error) {
	token := headers.Get("Authorization")
	if token == "" {
		return "", errors.New("No authorization in header")
	}
	split_token := strings.Split(token, " ")
	return split_token[1], nil
}
