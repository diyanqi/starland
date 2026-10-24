package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func (s *Store) ListPhotos(ctx context.Context, clientID string, cursor int64, limit int) ([]Photo, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.url, p.kind, p.caption, p.nickname, p.likes, p.created_at,
		       EXISTS (
		           SELECT 1 FROM photo_likes l
		           WHERE l.photo_id = p.id AND l.client_id = $1
		       ) AS liked
		FROM photos p
		WHERE $2::bigint = 0 OR p.id < $2
		ORDER BY p.id DESC
		LIMIT $3`, clientID, cursor, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	photos := make([]Photo, 0, limit)
	for rows.Next() {
		var p Photo
		if err := rows.Scan(&p.ID, &p.URL, &p.Kind, &p.Caption, &p.Nickname, &p.Likes, &p.CreatedAt, &p.Liked); err != nil {
			return nil, err
		}
		photos = append(photos, p)
	}
	return photos, rows.Err()
}

func (s *Store) CreatePhoto(ctx context.Context, url, caption, nickname, kind string) (Photo, error) {
	var p Photo
	err := s.pool.QueryRow(ctx, `
		INSERT INTO photos (url, kind, caption, nickname)
		VALUES ($1, $2, $3, $4)
		RETURNING id, url, kind, caption, nickname, likes, created_at`,
		url, kind, caption, nickname,
	).Scan(&p.ID, &p.URL, &p.Kind, &p.Caption, &p.Nickname, &p.Likes, &p.CreatedAt)
	return p, err
}

// LikePhoto 使用 photo_likes 表做“(照片, 客户端)”幂等去重。
// 同一客户端重复点赞不会重复计数，返回当前总数与是否已点过。
func (s *Store) LikePhoto(ctx context.Context, photoID int64, clientID string) (int, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback(ctx)

	var inserted int64
	err = tx.QueryRow(ctx, `
		INSERT INTO photo_likes (photo_id, client_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
		RETURNING photo_id`, photoID, clientID).Scan(&inserted)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// 已经点过赞，只读取当前计数。
		var likes int
		if err := tx.QueryRow(ctx, `SELECT likes FROM photos WHERE id = $1`, photoID).Scan(&likes); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return 0, false, ErrNotFound
			}
			return 0, false, err
		}
		return likes, false, tx.Commit(ctx)
	case err != nil:
		// 外键约束失败说明照片不存在。
		if isForeignKeyViolation(err) {
			return 0, false, ErrNotFound
		}
		return 0, false, err
	}

	var likes int
	if err := tx.QueryRow(ctx, `UPDATE photos SET likes = likes + 1 WHERE id = $1 RETURNING likes`, photoID).Scan(&likes); err != nil {
		return 0, false, err
	}
	return likes, true, tx.Commit(ctx)
}

func (s *Store) DeletePhoto(ctx context.Context, id int64) (string, error) {
	var url string
	err := s.pool.QueryRow(ctx, `DELETE FROM photos WHERE id = $1 RETURNING url`, id).Scan(&url)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return url, err
}
