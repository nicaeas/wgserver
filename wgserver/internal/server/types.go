package server

import (
	"time"
	t "wgserver/internal/types"
)

// Incoming message generic envelope
// We accept flexible keys; using map[string]any for raw, then try structured.

// MsgType moved to internal/types

type DailyTaskMessage = t.DailyTaskMessage

type Heartbeat struct {
	Type     string `json:"type"`
	ClientID string `json:"client_id"`
}

type HeartbeatResp struct {
	Type     string `json:"type"`
	ClientID string `json:"client_id"`
	Status   string `json:"status"`
}

type ConnectionAck struct {
	Code     int    `json:"code"`
	Message  string `json:"Message"`
	Type     string `json:"type"`
	ClientID string `json:"client_id"`
}

type AckReceived struct {
	ClientID string `json:"client_id"`
	Status   string `json:"status"`
}

// Role attribute payload (primary fields only; many optional)
// Field names are Chinese; use struct tags accordingly.

type RoleAttributes = t.RoleAttributes
type EquipItem = t.EquipItem
type Item = t.Item

// Map assignment push

type MapAssignment struct {
	RoleName string `json:"角色名"`
	Data     struct {
		Map   string `json:"地图"`
		Floor int    `json:"层数,omitempty"`
	} `json:"data"`
	ClientID string `json:"client_id"`
}

// Exchange transaction tracking (server-side)

type ExchangeState struct {
	Owner struct {
		RoleName string    `json:"角色名"`
		Status   string    `json:"状态"`
		When     time.Time `json:"确认时间"`
	} `json:"拥有者"`
	Receiver struct {
		RoleName string    `json:"角色名"`
		Status   string    `json:"状态"`
		When     time.Time `json:"确认时间"`
	} `json:"接收者"`
	Status string    `json:"总体状态"`
	Create time.Time `json:"创建时间"`
}
