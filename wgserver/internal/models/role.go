package models

import "time"

type Role struct {
	ID         int64     `db:"id"`
	RoleName   string    `db:"role_name"`
	Zone       string    `db:"zone"`
	MergeState string    `db:"merge_state"`
	Class      string    `db:"class"`
	School     string    `db:"school"`
	Skill      int       `db:"skill"`
	Level      int       `db:"level"`
	Lucky      int       `db:"lucky"`
	Magic      int       `db:"magic"`
	CurrentMap string    `db:"current_map"`
	ClientID   string    `db:"client_id"`
	CreatedAt  time.Time `db:"created_at"`
	X          int       `db:"x"`
	Y          int       `db:"y"`
	UpdatedAt  time.Time `db:"updated_at"`
}
