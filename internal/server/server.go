package server

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"wgserver/internal/logger"
	"wgserver/internal/services/alloc"
	eq "wgserver/internal/services/equipment"
	"wgserver/internal/services/roles"
	"wgserver/internal/services/tasks"
	msgtypes "wgserver/internal/types"

	"github.com/gorilla/websocket"
)

type Hub struct {
	clients   map[string]*Client
	clientsMu sync.RWMutex
	upgrader  websocket.Upgrader

	// heartbeats
	hbInterval   time.Duration
	maxHBNoReply time.Duration
}

var defaultHub *Hub

// 记录上一次对某个 充值区服|角色 推送的副本目标，用于变更日志去重
var lastAssign = struct {
	mu sync.Mutex
	m  map[string]alloc.MapTarget
}{m: map[string]alloc.MapTarget{}}

const (
	planRecalcInterval    = 3 * time.Hour
	planBroadcastInterval = time.Minute
)

type zonePlanState struct {
	Assignments []alloc.Assignment
	LastPlan    time.Time
	LastSend    time.Time
}

var planStates = struct {
	mu sync.RWMutex
	m  map[string]*zonePlanState
}{m: map[string]*zonePlanState{}}

func updatePlanState(zone string, assignments []alloc.Assignment, planTime time.Time) {
	copied := make([]alloc.Assignment, len(assignments))
	copy(copied, assignments)
	planStates.mu.Lock()
	defer planStates.mu.Unlock()
	state := planStates.m[zone]
	if state == nil {
		state = &zonePlanState{}
		planStates.m[zone] = state
	}
	state.Assignments = copied
	state.LastPlan = planTime
	state.LastSend = time.Time{}
}

func markPlanSent(zone string, sentAt time.Time) {
	planStates.mu.Lock()
	if state := planStates.m[zone]; state != nil {
		state.LastSend = sentAt
	}
	planStates.mu.Unlock()
}

func getPlanStateSnapshot(zone string) (assignments []alloc.Assignment, lastPlan, lastSend time.Time, ok bool) {
	planStates.mu.RLock()
	state, ok := planStates.m[zone]
	if !ok {
		planStates.mu.RUnlock()
		return nil, time.Time{}, time.Time{}, false
	}
	copied := make([]alloc.Assignment, len(state.Assignments))
	copy(copied, state.Assignments)
	lastPlan = state.LastPlan
	lastSend = state.LastSend
	planStates.mu.RUnlock()
	return copied, lastPlan, lastSend, true
}

func clearPlanState(zone string) {
	planStates.mu.Lock()
	delete(planStates.m, zone)
	planStates.mu.Unlock()
}

func NewHub(cfg interface{}) *Hub {
	defaultHub = &Hub{
		clients: make(map[string]*Client),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
		hbInterval:   30 * time.Second,
		maxHBNoReply: 3 * time.Minute,
	}
	// inject sender for tasks
	tasks.SetSender(SendJSON)
	// inject sender for equipment exchanges
	eq.SetSender(SendJSON)
	// global planner loop: keep minute broadcasts and 3h replans per zone
	go func() {
		t := time.NewTicker(planBroadcastInterval)
		defer t.Stop()
		for tick := range t.C {
			zones := roles.Instance().ListZones()
			for _, z := range zones {
				snap := roles.Instance().SnapshotZone(z)
				if len(snap.Roles) == 0 {
					clearPlanState(z)
					continue
				}
				mergeState := mergeStateFromSnapshot(snap)
				need := neededByMerge(mergeState)
				thresholdMet := len(snap.Roles) >= need
				waitExpired := tick.After(snap.WaitAllocUntil)

				assignments, lastPlan, lastSend, hasState := getPlanStateSnapshot(z)
				shouldPlan := false
				if !hasState {
					if thresholdMet || waitExpired {
						shouldPlan = true
					}
				} else if tick.Sub(lastPlan) >= planRecalcInterval {
					shouldPlan = true
				}

				if shouldPlan {
					assignments = alloc.Plan(z)
					updatePlanState(z, assignments, tick)
					lastSend = time.Time{}
				}

				if len(assignments) == 0 {
					continue
				}

				if shouldPlan || tick.Sub(lastSend) >= planBroadcastInterval {
					dispatchAssignments(snap, assignments)
					markPlanSent(z, time.Now())
				}
			}
		}
	}()
	return defaultHub
}
func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	id := h.newClientID()
	c := &Client{ID: id, Conn: conn, Send: make(chan []byte, 64), LastHBAt: time.Now()}
	h.clientsMu.Lock()
	h.clients[id] = c
	h.clientsMu.Unlock()
	// 连接事件：记录当前总连接数
	h.clientsMu.RLock()
	total := len(h.clients)
	h.clientsMu.RUnlock()
	logger.Connection().Printf("connected client_id=%s from %s total=%d", id, r.RemoteAddr, total)
	ack := ConnectionAck{Code: 200, Message: "成功", Type: string(msgtypes.MsgTypeConnectionAck), ClientID: id}
	_ = conn.WriteJSON(ack)

	go h.writer(c)
	go h.reader(c)
	go h.heartbeatSender(c)
}

func (h *Hub) newClientID() string {
	return strings.ToUpper(randStr(8)) + "-" + strings.ToUpper(randStr(4))
}

func randStr(n int) string {
	letters := []rune("0123456789abcdefghijklmnopqrstuvwxyz")
	s := make([]rune, n)
	for i := range s {
		s[i] = letters[rand.Intn(len(letters))]
	}
	return string(s)
}

func (h *Hub) reader(c *Client) {
	defer h.disconnect(c)
	c.Conn.SetReadLimit(1 << 20)
	for {
		mt, data, err := c.Conn.ReadMessage()
		if err != nil {
			return
		}
		if mt != websocket.TextMessage {
			continue
		}
		// try detect heartbeat response
		var base map[string]any
		if err := json.Unmarshal(data, &base); err == nil {
			if typ, ok := base["type"].(string); ok && typ == string(msgtypes.MsgTypeHeartbeatResponse) {
				c.LastHBAt = time.Now()
				continue
			}
			if st, ok := base["status"].(string); ok && st == "received" {
				// 通用ACK：记录并标注该客户端关联角色的分配已被确认
				logger.Connection().Printf("ack received from client_id=%s", c.ID)
				onAckReceived(c.ID)
				continue
			}
		}
		// handle other messages
		h.handleMessage(c, data)
	}
}

func (h *Hub) writer(c *Client) {
	defer h.disconnect(c)
	for msg := range c.Send {
		if err := c.SafeWrite(msg); err != nil {
			return
		}
	}
}

func (h *Hub) disconnect(c *Client) {
	c.mu.Lock()
	_ = c.Conn.Close()
	c.mu.Unlock()

	h.clientsMu.Lock()
	delete(h.clients, c.ID)
	h.clientsMu.Unlock()

	// 清理该客户端角色信息（调用角色服务进行清理）
	onClientDisconnected(c.ID)
	h.clientsMu.RLock()
	total := len(h.clients)
	h.clientsMu.RUnlock()
	logger.Connection().Printf("disconnected client_id=%s total=%d", c.ID, total)
}

func (h *Hub) heartbeatSender(c *Client) {
	t := time.NewTicker(h.hbInterval)
	defer t.Stop()
	for range t.C {
		// send heartbeat
		hb := Heartbeat{Type: string(msgtypes.MsgTypeHeartbeat), ClientID: c.ID}
		b, _ := json.Marshal(hb)
		select {
		case c.Send <- b:
		default:
		}
		// check stale
		if time.Since(c.LastHBAt) > h.maxHBNoReply {
			logger.Connection().Printf("client_id=%s heartbeat timeout; closing", c.ID)
			c.Conn.Close()
			return
		}
	}
}

func (h *Hub) handleMessage(c *Client, data []byte) {
	// dispatch by content
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return
	}

	// 日常任务
	if mt, ok := obj["消息类型"].(string); ok && mt == string(msgtypes.MsgTypeDailyTask) {
		h.handleDailyTaskMessage(c, data)
		return
	}
	// 装备交换确认/坐标/等 按字段判断
	if _, ok := obj["操作"].(string); ok {
		h.handleExchangeConfirmation(c, data)
		return
	}
	if _, ok := obj["地图"].(string); ok && obj["X"] != nil && obj["Y"] != nil && obj["来源角色"] != nil {
		h.handleExchangeCoordinate(c, data)
		return
	}
	// 角色属性
	h.handleRoleAttributes(c, data)
}

// stubs wired to services (implemented in other files)
func (h *Hub) handleDailyTaskMessage(c *Client, data []byte) {
	if msg, ok := tasks.ParseDailyTaskMessage(data); ok {
		tasks.Instance().Handle(msg)
	}
}
func (h *Hub) handleExchangeConfirmation(c *Client, data []byte) {
	// 将确认消息交给装备交换管理器，遍历区服匹配
	zones := roles.Instance().ListZones()
	for _, z := range zones {
		eq.HandleConfirm(z, data)
	}
}
func (h *Hub) handleExchangeCoordinate(c *Client, data []byte) {
	zones := roles.Instance().ListZones()
	for _, z := range zones {
		eq.HandleCoordinate(z, data)
	}
}
func (h *Hub) handleRoleAttributes(c *Client, data []byte) {
	info, _ := roles.Instance().UpsertRole(data)
	if info == nil {
		return
	}
	// trigger planning when role count sufficient or when wait deadline passed
	snap := roles.Instance().SnapshotZone(info.Zone)
	need := neededByMerge(info.MergeState)
	if len(snap.Roles) >= need || time.Now().After(snap.WaitAllocUntil) {
		planTime := time.Now()
		as := alloc.Plan(info.Zone)
		updatePlanState(info.Zone, as, planTime)
		if len(as) > 0 {
			dispatchAssignments(snap, as)
			markPlanSent(info.Zone, time.Now())
		}
		// 同步触发装备分配与交换事务
		eq.PlanAndDispatch(info.Zone)
	}
}

// called on disconnect to cleanup roles owned by client
func onClientDisconnected(clientID string) { roles.Instance().RemoveClient(clientID) }

// 标注ACK：将该client_id关联的所有角色分配视为已被确认（仅做日志记录，周期间仍会按策略推送）
func onAckReceived(clientID string) {
	zones := roles.Instance().ListZones()
	for _, z := range zones {
		snap := roles.Instance().SnapshotZone(z)
		for role, cid := range snap.ClientByRole {
			if cid == clientID {
				logger.MapAlloc().Printf("ack confirmed zone=%s role=%s client_id=%s", z, role, clientID)
			}
		}
	}
}

func neededByMerge(ms string) int {
	switch ms {
	case "未合区":
		return 12
	case "一合", "一合区", "第一次合区":
		return 24
	case "二合", "三合", "四合", "五合", "六合":
		return 48
	case "七合", "七合以后":
		return 28
	default:
		return 12
	}
}

// Global accessors
func HubInstance() *Hub { return defaultHub }

// Send JSON to a client by id (non-blocking)
func SendJSON(clientID string, v any) {
	h := HubInstance()
	if h == nil {
		return
	}
	h.clientsMu.RLock()
	c := h.clients[clientID]
	h.clientsMu.RUnlock()
	if c == nil {
		return
	}
	b, _ := json.Marshal(v)
	select {
	case c.Send <- b:
	default:
	}
}

// 仅在分配目标变化时记录： 角色名 原先副本地图----->规划副本地图
func logMapAssignIfChanged(snap *roles.ZoneState, role string, target alloc.MapTarget) {
	// zone 取自该角色所在快照（ClientByRole 的 key 未显式包含 zone，这里取一个快照字段不足以得到 zone 名，
	// 因此从角色对象读取充值区服更稳妥）
	zone := ""
	if rr, ok := snap.Roles[role]; ok {
		zone = rr.Zone
	}
	key := zone + "|" + role
	lastAssign.mu.Lock()
	prev, ok := lastAssign.m[key]
	same := ok && prev.Map == target.Map && prev.Floor == target.Floor
	if !same {
		prevStr := prev.Map
		if prev.Floor > 0 {
			prevStr = prevStr + formatFloor(prev.Floor)
		}
		nowStr := target.Map
		if target.Floor > 0 {
			nowStr = nowStr + formatFloor(target.Floor)
		}
		logger.MapAlloc().Printf("role=%s prev=%s -> plan=%s", role, nonEmpty(prevStr, "(无)"), nowStr)
		lastAssign.m[key] = target
	}
	lastAssign.mu.Unlock()
}

func formatFloor(f int) string { return "-" + itoa(f) }
func dispatchAssignments(snap *roles.ZoneState, assignments []alloc.Assignment) {
	for _, a := range assignments {
		cid := snap.ClientByRole[a.RoleName]
		if cid == "" {
			continue
		}
		msg := MapAssignment{RoleName: a.RoleName, ClientID: cid}
		msg.Data.Map = a.Target.Map
		if ri, ok := snap.Roles[a.RoleName]; ok && ri.Class == "法师" && a.Target.Floor > 0 {
			msg.Data.Floor = a.Target.Floor
		}
		logMapAssignIfChanged(snap, a.RoleName, a.Target)
		SendJSON(cid, msg)
	}
}

func mergeStateFromSnapshot(zs *roles.ZoneState) string {
	for _, r := range zs.Roles {
		if r.MergeState != "" {
			return r.MergeState
		}
	}
	return "未合区"
}

func itoa(v int) string { return fmt.Sprintf("%d", v) }
func nonEmpty(s, alt string) string {
	if s == "" {
		return alt
	}
	return s
}
