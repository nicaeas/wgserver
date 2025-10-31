package equipment

import (
	"encoding/json"
	"sync"
	"time"

	"wgserver/internal/db"
	"wgserver/internal/logger"
	rm "wgserver/internal/services/roles"

	"github.com/jmoiron/sqlx"
)

type senderFn func(clientID string, payload any)

var send senderFn

func SetSender(fn senderFn) { send = fn }

// Exchange state machine ------------------------------------------------------

type exchKey struct{ Zone, Owner, Receiver, Item string }

type exchState struct {
	OwnerOK    bool
	ReceiverOK bool
	CreateAt   time.Time
}

var (
	exMu  sync.Mutex
	exMap = map[exchKey]*exchState{}
)

func startExchange(z, owner, receiver, item string) {
	exMu.Lock()
	defer exMu.Unlock()
	k := exchKey{Zone: z, Owner: owner, Receiver: receiver, Item: item}
	if _, ok := exMap[k]; ok {
		return
	}
	exMap[k] = &exchState{CreateAt: time.Now()}
	_ = db.Tx(func(tx *sqlx.Tx) error {
		_, err := tx.Exec(`INSERT INTO exchanges (zone, owner_role, receiver_role, item_name, status) VALUES (?,?,?,?, 'waiting')`, z, owner, receiver, item)
		return err
	})
}

func markOwnerOK(z, owner, receiver, item string) {
	exMu.Lock()
	defer exMu.Unlock()
	k := exchKey{Zone: z, Owner: owner, Receiver: receiver, Item: item}
	st := exMap[k]
	if st == nil {
		st = &exchState{CreateAt: time.Now()}
		exMap[k] = st
	}
	st.OwnerOK = true
	_ = db.Tx(func(tx *sqlx.Tx) error {
		_, err := tx.Exec(`UPDATE exchanges SET status='owner_ok' WHERE zone=? AND owner_role=? AND receiver_role=? AND item_name=?`, z, owner, receiver, item)
		return err
	})
	checkDoneLocked(k, st)
}

func markReceiverOK(z, owner, receiver, item string) {
	exMu.Lock()
	defer exMu.Unlock()
	k := exchKey{Zone: z, Owner: owner, Receiver: receiver, Item: item}
	st := exMap[k]
	if st == nil {
		st = &exchState{CreateAt: time.Now()}
		exMap[k] = st
	}
	st.ReceiverOK = true
	_ = db.Tx(func(tx *sqlx.Tx) error {
		_, err := tx.Exec(`UPDATE exchanges SET status='receiver_ok' WHERE zone=? AND owner_role=? AND receiver_role=? AND item_name=?`, z, owner, receiver, item)
		return err
	})
	checkDoneLocked(k, st)
}

func checkDoneLocked(k exchKey, st *exchState) {
	if st.OwnerOK && st.ReceiverOK {
		_ = db.Tx(func(tx *sqlx.Tx) error {
			_, err := tx.Exec(`UPDATE exchanges SET status='done' WHERE zone=? AND owner_role=? AND receiver_role=? AND item_name=?`, k.Zone, k.Owner, k.Receiver, k.Item)
			return err
		})
		// 通知双方交换完成
		snap := rm.Instance().SnapshotZone(k.Zone)
		ocid := snap.ClientByRole[k.Owner]
		rcid := snap.ClientByRole[k.Receiver]
		msg := func(role, partner, cid string) map[string]any {
			return map[string]any{"角色名": role, "交换伙伴": partner, "装备名称": k.Item, "状态": "交换成功", "client_id": cid}
		}
		if send != nil {
			send(ocid, msg(k.Owner, k.Receiver, ocid))
			send(rcid, msg(k.Receiver, k.Owner, rcid))
		}
		// 装备分配成功日志（确认成功才记录）：对两个角色分别记录该物品的变更
		logger.Equipment().Printf("zone=%s role=%s equip_change: %s -> (已转出)", k.Zone, k.Owner, k.Item)
		logger.Equipment().Printf("zone=%s role=%s equip_change: (获得) <- %s", k.Zone, k.Receiver, k.Item)
		delete(exMap, k)
	}
}

// External handlers from server ---------------------------------------------

type ConfirmPayload struct {
	RoleName string `json:"角色名"`
	Op       string `json:"操作"` // 装备转移/装备接收
	Item     string `json:"装备名称"`
	Status   string `json:"状态"`
	ClientID string `json:"client_id"`
}

type CoordPayload struct {
	RoleName string `json:"角色名"`
	FromRole string `json:"来源角色"`
	Map      string `json:"地图"`
	X        int    `json:"X"`
	Y        int    `json:"Y"`
	ClientID string `json:"client_id"`
}

func HandleConfirm(zone string, data []byte) {
	var p ConfirmPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return
	}
	if p.Status != "成功" {
		return
	}
	// 查找对应的 key：谁和谁、哪件
	// 需要从 roles 快照反推另一方：由于客户端会带来源角色/或在我们制作交易时已记录，这里假设我们在生成时已唯一映射（简化：在确认时通过遍历 exMap 匹配）
	exMu.Lock()
	var target *exchKey
	for k := range exMap {
		if k.Zone != zone {
			continue
		}
		if k.Owner == p.RoleName && k.Item == p.Item {
			target = &k
			break
		}
		if k.Receiver == p.RoleName && k.Item == p.Item {
			target = &k
			break
		}
	}
	exMu.Unlock()
	if target == nil {
		return
	}
	if p.RoleName == target.Owner && p.Op == "装备转移" {
		markOwnerOK(zone, target.Owner, target.Receiver, target.Item)
	} else if p.RoleName == target.Receiver && p.Op == "装备接收" {
		markReceiverOK(zone, target.Owner, target.Receiver, target.Item)
	}
}

func HandleCoordinate(zone string, data []byte) {
	var c CoordPayload
	if err := json.Unmarshal(data, &c); err != nil {
		return
	}
	// 把坐标转发给来源角色（拥有者）
	snap := rm.Instance().SnapshotZone(zone)
	ownerCID := snap.ClientByRole[c.FromRole]
	if ownerCID == "" || send == nil {
		return
	}
	payload := map[string]any{"角色名": c.RoleName, "地图": c.Map, "X": c.X, "Y": c.Y, "client_id": ownerCID}
	send(ownerCID, payload)
}

// Planner + Dispatch ---------------------------------------------------------

// 触发分配：比较当前拥有者与目标搭配，生成需要的转移并下发交换流程
func PlanAndDispatch(zone string) {
	plan := PlanZone(zone)
	snap := rm.Instance().SnapshotZone(zone)
	if send == nil {
		return
	}

	// 构建现有持有映射：item -> owner role (多件按多条记录)
	ownerByItem := map[string][]string{}
	for _, r := range snap.Roles {
		for _, e := range r.Equipments {
			ownerByItem[e.Name] = append(ownerByItem[e.Name], r.RoleName)
		}
		for _, it := range r.Backpack {
			for i := 0; i < it.Count; i++ {
				ownerByItem[it.Name] = append(ownerByItem[it.Name], r.RoleName)
			}
		}
		for _, it := range r.Warehouse {
			for i := 0; i < it.Count; i++ {
				ownerByItem[it.Name] = append(ownerByItem[it.Name], r.RoleName)
			}
		}
	}

	// 为每个角色计算需要的物品名集合，与当前相比找差集
	for roleName, outfit := range plan {
		need := map[string]int{}
		for _, name := range outfit.BySlot {
			if name != "" {
				need[name]++
			}
		}
		// 减去该角色已拥有（任意位置）的数量
		cur := map[string]int{}
		if rr, ok := snap.Roles[roleName]; ok {
			for _, e := range rr.Equipments {
				cur[e.Name]++
			}
			for _, it := range rr.Backpack {
				cur[it.Name] += it.Count
			}
			for _, it := range rr.Warehouse {
				cur[it.Name] += it.Count
			}
		}
		for name, c := range cur {
			if need[name] > 0 {
				need[name] -= min(need[name], c)
			}
		}
		// 对 need > 0 的物品，从 ownerByItem 中选择拥有者，发起交换
		for name, n := range need {
			for x := 0; x < n; x++ {
				owners := ownerByItem[name]
				var owner string
				for i, o := range owners {
					if o == roleName {
						continue
					} // 不从自己拿
					owner = o
					// 移除已用掉的这件映射
					ownerByItem[name] = append(owners[:i], owners[i+1:]...)
					break
				}
				if owner == "" {
					continue
				}
				// 下发分配与接收指令
				ocid := snap.ClientByRole[owner]
				rcid := snap.ClientByRole[roleName]
				if ocid == "" || rcid == "" {
					continue
				}
				ownerMsg := map[string]any{"角色名": owner, "目标角色": roleName, "装备名称": name, "client_id": ocid}
				recvMsg := map[string]any{"角色名": roleName, "来源角色": owner, "装备名称": name, "client_id": rcid}
				send(ocid, ownerMsg)
				send(rcid, recvMsg)
				startExchange(zone, owner, roleName, name)
				logger.Equipment().Printf("dispatch exchange zone=%s owner=%s receiver=%s item=%s", zone, owner, roleName, name)
			}
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
