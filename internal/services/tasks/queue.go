package tasks

import (
	"encoding/json"
	"sync"

	"wgserver/internal/logger"
	"wgserver/internal/services/roles"
	t "wgserver/internal/types"
)

type Queue struct {
	mu sync.Mutex
	// zone -> running set (max 3) and FIFO queue
	running map[string]map[string]struct{}
	queue   map[string][]string
	// 最近一次状态：zone -> role -> status
	status map[string]map[string]string
	sender func(clientID string, payload any)
}

var qinst *Queue
var once sync.Once

func Instance() *Queue {
	once.Do(func() {
		qinst = &Queue{running: map[string]map[string]struct{}{}, queue: map[string][]string{}, status: map[string]map[string]string{}}
	})
	return qinst
}

// SetSender sets the function used to send JSON payloads to clients
func SetSender(fn func(clientID string, payload any)) { Instance().sender = fn }

func (q *Queue) Handle(msg t.DailyTaskMessage) {
	zone := msg.Zone
	role := msg.RoleName
	q.mu.Lock()
	defer q.mu.Unlock()
	rset := q.running[zone]
	if rset == nil {
		rset = map[string]struct{}{}
		q.running[zone] = rset
	}
	if q.status[zone] == nil {
		q.status[zone] = map[string]string{}
	}
	switch msg.TaskStatus {
	case "开始":
		// 幂等处理：如果已在运行，直接重发允许但不重复入队
		if _, ok := rset[role]; ok {
			q.sendStatus(msg.ClientID, role, zone, "允许")
			return
		}
		// 如果已在队列，直接重发等待
		if contains(q.queue[zone], role) {
			q.sendStatus(msg.ClientID, role, zone, "等待")
			return
		}
		if len(rset) < 3 {
			rset[role] = struct{}{}
			q.sendStatus(msg.ClientID, role, zone, "允许")
		} else {
			q.queue[zone] = append(q.queue[zone], role)
			q.sendStatus(msg.ClientID, role, zone, "等待")
		}
	case "完成":
		delete(rset, role)
		// 记录完成状态变化
		q.sendStatus(msg.ClientID, role, zone, "完成")
		// schedule next
		if list := q.queue[zone]; len(list) > 0 && len(rset) < 3 {
			next := list[0]
			q.queue[zone] = list[1:]
			rset[next] = struct{}{}
			// We need client_id of next role: this comes via roles manager snapshot
			cid := roleClientID(zone, next)
			q.sendStatus(cid, next, zone, "允许")
		}
	}
}

func (q *Queue) sendStatus(clientID, role, zone, status string) {
	// 去重：仅在状态变化时记录
	if q.status[zone] == nil {
		q.status[zone] = map[string]string{}
	}
	prev := q.status[zone][role]
	if prev == status {
		// 状态未变，仅重发消息（如果需要）
		if q.sender != nil {
			q.sender(clientID, map[string]any{"角色名": role, "充值区服": zone, "消息类型": string(t.MsgTypeDailyTask), "任务状态": status, "client_id": clientID})
		}
		return
	}
	q.status[zone][role] = status
	resp := map[string]any{
		"角色名":       role,
		"充值区服":      zone,
		"消息类型":      string(t.MsgTypeDailyTask),
		"任务状态":      status,
		"client_id": clientID,
	}
	if q.sender != nil {
		q.sender(clientID, resp)
	}
	logger.TaskQueue().Printf("zone=%s role=%s status=%s", zone, role, status)
}

func roleClientID(zone, role string) string {
	snap := roles.Instance().SnapshotZone(zone)
	return snap.ClientByRole[role]
}

func ParseDailyTaskMessage(data []byte) (t.DailyTaskMessage, bool) {
	var m t.DailyTaskMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return m, false
	}
	if m.MsgType != string(t.MsgTypeDailyTask) {
		return m, false
	}
	return m, true
}

func contains(arr []string, s string) bool {
	for _, v := range arr {
		if v == s {
			return true
		}
	}
	return false
}
