package roles

import (
	"encoding/json"
	"sync"
	"time"

	"wgserver/internal/db"
	"wgserver/internal/logger"
	t "wgserver/internal/types"

	"github.com/jmoiron/sqlx"
)

type RoleInfo struct{ t.RoleAttributes }

type ZoneState struct {
	Roles          map[string]*RoleInfo // role_name -> info
	ClientByRole   map[string]string    // role -> client_id
	LastUpdate     time.Time
	WaitAllocUntil time.Time // deadline to wait for more roles (3min)
}

type Manager struct {
	mu    sync.RWMutex
	zones map[string]*ZoneState
}

var singleton *Manager
var once sync.Once

func Instance() *Manager {
	once.Do(func() { singleton = &Manager{zones: make(map[string]*ZoneState)} })
	return singleton
}

func (m *Manager) UpsertRole(raw []byte) (*RoleInfo, bool) {
	var r t.RoleAttributes
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, false
	}
	z := r.Zone
	if z == "" || r.RoleName == "" {
		return nil, false
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	zs := m.zones[z]
	if zs == nil {
		zs = &ZoneState{Roles: map[string]*RoleInfo{}, ClientByRole: map[string]string{}}
		m.zones[z] = zs
	}

	prev, exists := zs.Roles[r.RoleName]
	// 变更检测：仅当新角色，或当前地图/装备发生变化时记录 role_info
	shouldLog := !exists
	if exists {
		// 地图变化
		if prev.MapName != r.MapName {
			shouldLog = true
		}
		// 装备变化
		if !equipEqual(prev.Equipments, r.Equipments) {
			shouldLog = true
		}
	}
	zs.Roles[r.RoleName] = &RoleInfo{RoleAttributes: r}
	zs.ClientByRole[r.RoleName] = r.ClientID
	zs.LastUpdate = time.Now()
	// 滑动窗口：每次有新角色或属性更新，若仍未达到阈值，将等待截止时间向后推 3 分钟；
	// 是否达到阈值的判定由上层 server 在推送/分配前进行，因此这里无须了解阈值具体数值
	zs.WaitAllocUntil = time.Now().Add(3 * time.Minute)

	// persist new/changed role to DB, and log only when new or map/equipment changed (TODO: diff detection)
	saveRole(&r)
	if shouldLog {
		logger.RoleInfo().Printf("role=%s zone=%s merge=%s class=%s school=%s magic=%d lucky=%d level=%d skill=%d map=%s",
			r.RoleName, r.Zone, r.MergeState, r.Class, r.School, r.Magic, r.Lucky, r.Level, r.Skill, r.MapName)
	}
	return zs.Roles[r.RoleName], !exists
}

func (m *Manager) RemoveClient(clientID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, zs := range m.zones {
		for role, cid := range zs.ClientByRole {
			if cid == clientID {
				delete(zs.Roles, role)
				delete(zs.ClientByRole, role)
			}
		}
	}
}

func (m *Manager) SnapshotZone(zone string) *ZoneState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if zs, ok := m.zones[zone]; ok {
		copy := &ZoneState{Roles: make(map[string]*RoleInfo), ClientByRole: make(map[string]string), LastUpdate: zs.LastUpdate, WaitAllocUntil: zs.WaitAllocUntil}
		for k, v := range zs.Roles {
			vv := *v
			copy.Roles[k] = &vv
		}
		for k, v := range zs.ClientByRole {
			copy.ClientByRole[k] = v
		}
		return copy
	}
	return &ZoneState{Roles: map[string]*RoleInfo{}, ClientByRole: map[string]string{}}
}

func (m *Manager) ListZones() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.zones))
	for z := range m.zones {
		out = append(out, z)
	}
	return out
}

func saveRole(r *t.RoleAttributes) {
	_ = db.Tx(func(tx *sqlx.Tx) error {
		_, err := tx.Exec(`INSERT INTO roles (role_name, zone, merge_state, class, school, skill, level, lucky, magic, current_map, client_id, created_at, x, y)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE merge_state=VALUES(merge_state), class=VALUES(class), school=VALUES(school), skill=VALUES(skill), level=VALUES(level), lucky=VALUES(lucky), magic=VALUES(magic), current_map=VALUES(current_map), client_id=VALUES(client_id), x=VALUES(x), y=VALUES(y)`,
			r.RoleName, r.Zone, r.MergeState, r.Class, r.School, r.Skill, r.Level, r.Lucky, r.Magic, r.MapName, r.ClientID, r.CreatedAt, r.X, r.Y)
		return err
	})
}

// 判断装备集合是否相同（按装备名+部位去重）；不关注顺序
func equipEqual(a, b []t.EquipItem) bool {
	if len(a) != len(b) {
		// 可能位置名称不同但数量相同，这里仍需集合比较
	}
	set := func(list []t.EquipItem) map[string]int {
		m := map[string]int{}
		for _, e := range list {
			key := e.Slot + "|" + e.Name
			m[key]++
		}
		return m
	}
	sa, sb := set(a), set(b)
	if len(sa) != len(sb) {
		return false
	}
	for k, va := range sa {
		if vb, ok := sb[k]; !ok || vb != va {
			return false
		}
	}
	return true
}
