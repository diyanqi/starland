package store

import "time"

type Photo struct {
	ID        int64     `json:"id"`
	URL       string    `json:"url"`
	Kind      string    `json:"kind"`
	Caption   string    `json:"caption"`
	Nickname  string    `json:"nickname"`
	Likes     int       `json:"likes"`
	Liked     bool      `json:"liked"`
	CreatedAt time.Time `json:"createdAt"`
}

type Message struct {
	ID         int64     `json:"id"`
	Content    string    `json:"content"`
	Type       string    `json:"type"`
	Nickname   string    `json:"nickname"`
	ReplyCount int       `json:"replyCount"`
	CreatedAt  time.Time `json:"createdAt"`
}

type Reply struct {
	ID        int64     `json:"id"`
	MessageID int64     `json:"messageId"`
	Content   string    `json:"content"`
	Nickname  string    `json:"nickname"`
	CreatedAt time.Time `json:"createdAt"`
}
