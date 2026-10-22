package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

func (s *Store) ListMessages(ctx context.Context, msgType string, cursor int64, limit int) ([]Message, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT m.id, m.content, m.type, m.nickname, m.created_at,
		       COUNT(r.id)::int AS reply_count
		FROM messages m
		LEFT JOIN replies r ON r.message_id = m.id
		WHERE ($1 = '' OR m.type = $1)
		  AND ($2::bigint = 0 OR m.id < $2)
		GROUP BY m.id
		ORDER BY m.id DESC
		LIMIT $3`, msgType, cursor, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := make([]Message, 0, limit)
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.Content, &m.Type, &m.Nickname, &m.CreatedAt, &m.ReplyCount); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

func (s *Store) CreateMessage(ctx context.Context, content, msgType, nickname string) (Message, error) {
	var m Message
	err := s.pool.QueryRow(ctx, `
		INSERT INTO messages (content, type, nickname)
		VALUES ($1, $2, $3)
		RETURNING id, content, type, nickname, created_at`,
		content, msgType, nickname,
	).Scan(&m.ID, &m.Content, &m.Type, &m.Nickname, &m.CreatedAt)
	return m, err
}

func (s *Store) DeleteMessage(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM messages WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListReplies(ctx context.Context, messageID int64, limit int) ([]Reply, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM messages WHERE id = $1)`, messageID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, message_id, content, nickname, created_at
		FROM replies
		WHERE message_id = $1
		ORDER BY id ASC
		LIMIT $2`, messageID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	replies := make([]Reply, 0, 8)
	for rows.Next() {
		var r Reply
		if err := rows.Scan(&r.ID, &r.MessageID, &r.Content, &r.Nickname, &r.CreatedAt); err != nil {
			return nil, err
		}
		replies = append(replies, r)
	}
	return replies, rows.Err()
}

func (s *Store) CreateReply(ctx context.Context, messageID int64, content, nickname string) (Reply, error) {
	var r Reply
	err := s.pool.QueryRow(ctx, `
		INSERT INTO replies (message_id, content, nickname)
		VALUES ($1, $2, $3)
		RETURNING id, message_id, content, nickname, created_at`,
		messageID, content, nickname,
	).Scan(&r.ID, &r.MessageID, &r.Content, &r.Nickname, &r.CreatedAt)
	if isForeignKeyViolation(err) {
		return r, ErrNotFound
	}
	return r, err
}

func (s *Store) DeleteReply(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM replies WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
