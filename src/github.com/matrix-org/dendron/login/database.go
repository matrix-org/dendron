package login

import (
	"database/sql"
	"fmt"
	"log"
	"sync/atomic"
)

type database interface {
	canonicalUserIDAndPasswordHash(userID string) (string, string, error)
	insertTokens(userID, accessToken, refreshToken string) error
	matrixIDFor3PID(medium, address string) (string, error)
}

// An sqlDatabase for doing the SQL queries needed to login a user
type sqlDatabase struct {
	db                 *sql.DB
	nextAccessTokenID  int64
	nextRefreshTokenID int64
}

func makeSQLDatabase(db *sql.DB) (database, error) {
	accessTokenID, refreshTokenID, err := getNextIDs(db)
	if err != nil {
		return nil, err
	}

	log.Printf("Intitial minimum token ids are %d and %d", accessTokenID, refreshTokenID)

	return &sqlDatabase{db, accessTokenID, refreshTokenID}, nil
}

func (s *sqlDatabase) canonicalUserIDAndPasswordHash(userID string) (string, string, error) {
	row := s.db.QueryRow("SELECT name, password_hash FROM users WHERE lower(name) = lower($1)", userID)
	var canonicalID sql.NullString
	var hash sql.NullString
	if err := row.Scan(&canonicalID, &hash); err != nil {
		return "", "", err
	}

	if !canonicalID.Valid {
		return "", "", fmt.Errorf("canonicalID for %q was null", userID)
	}

	if !hash.Valid {
		return "", "", fmt.Errorf("password hash for %q was null", userID)
	}

	return canonicalID.String, hash.String, nil
}

func (s *sqlDatabase) insertTokens(userID, accessToken, refreshToken string) error {

	// Assumes that NextAccessTokenID and NextRefreshTokenID have been set to the
	// minimum value in the database and that only one instance of this login
	// handler is running against a given database
	accessTokenID := -atomic.AddInt64(&s.nextAccessTokenID, 1)
	refreshTokenID := -atomic.AddInt64(&s.nextRefreshTokenID, 1)

	txn, err := s.db.Begin()
	if err != nil {
		return nil
	}
	defer txn.Rollback()

	if _, err := txn.Exec("INSERT INTO access_tokens (id, user_id, token) VALUES ($1, $2, $3)", accessTokenID, userID, accessToken); err != nil {
		return err
	}

	if _, err := txn.Exec("INSERT INTO refresh_tokens (id, user_id, token) VALUES ($1, $2, $3)", refreshTokenID, userID, refreshToken); err != nil {
		return err
	}

	txn.Commit()
	return nil
}

func (s *sqlDatabase) matrixIDFor3PID(medium, address string) (string, error) {
	row := s.db.QueryRow("SELECT user_id FROM user_threepids WHERE medium = $1 AND address = $2", medium, address)
	var userID string
	err := row.Scan(&userID)
	return userID, err
}

func getNextIDs(db *sql.DB) (accessTokenID, refreshTokenID int64, err error) {
	var id sql.NullInt64

	row := db.QueryRow("SELECT min(id) FROM access_tokens")
	if err = row.Scan(&id); err != nil {
		return
	}

	if id.Valid {
		accessTokenID = -id.Int64
	}

	row = db.QueryRow("SELECT min(id) FROM refresh_tokens")
	if err = row.Scan(&id); err != nil {
		return
	}

	if id.Valid {
		refreshTokenID = -id.Int64
	}

	return
}
