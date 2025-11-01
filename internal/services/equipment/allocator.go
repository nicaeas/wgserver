package equipment

// NOTE: Complex allocation strategy per spec is large; this is a scaffold for future expansion.

import (
	"sort"

	t "wgserver/internal/types"
)

type PoolItem struct {
	Name  string
	Count int
	Owner string // role name if bound, else empty
	Slot  string
	Place string // equipped/bag/warehouse
}

type Allocation struct {
	RoleName string
	Items    []string // item names assigned (simplified)
}

// Build an equipment pool from all roles in a zone
func BuildPool(roles []t.RoleAttributes) []PoolItem {
	var pool []PoolItem
	for _, r := range roles {
		for _, e := range r.Equipments {
			pool = append(pool, PoolItem{Name: e.Name, Count: 1, Owner: r.RoleName, Slot: e.Slot, Place: "equipped"})
		}
		for _, it := range r.Backpack {
			pool = append(pool, PoolItem{Name: it.Name, Count: it.Count, Owner: r.RoleName, Place: "bag"})
		}
		for _, it := range r.Warehouse {
			pool = append(pool, PoolItem{Name: it.Name, Count: it.Count, Owner: r.RoleName, Place: "warehouse"})
		}
	}
	return pool
}

// 统计每个套装的可用4件套数量
func countSetFourPieceAvailability(pool []PoolItem) map[string]int {
	availability := make(map[string]int)
	
	// 统计每个套装的可用装备数量
	setItemCount := make(map[string]int)
	for _, p := range pool {
		for setName, items := range EquipmentSets {
			if _, exists := items[p.Name]; exists {
				setItemCount[setName]++
				break
			}
		}
	}
	
	// 计算每个套装的可用4件套数量（最多为可用装备数量除以4，向下取整）
	for setName, count := range setItemCount {
		availability[setName] = count / 4
	}
	
	return availability
}

// 按角色等级和道术排序角色
func sortRolesByLevelAndMagic(roles []t.RoleAttributes) {
	sort.SliceStable(roles, func(i, j int) bool {
		// 优先按等级排序
		if roles[i].Level != roles[j].Level {
			return roles[i].Level > roles[j].Level
		}
		// 等级相同按道术排序
		if roles[i].Magic != roles[j].Magic {
			return roles[i].Magic > roles[j].Magic
		}
		// 道术相同按角色名排序
		return roles[i].RoleName < roles[j].RoleName
	})
}

// EnsureFourPiece 尝试为每个角色分配4件套主套装
func EnsureFourPiece(roles []t.RoleAttributes, pool []PoolItem) []Allocation {
	// 按角色等级和道术排序
	sortRolesByLevelAndMagic(roles)

	allocs := make([]Allocation, 0, len(roles))
	// 统计每个套装的可用4件套数量
	setAvailability := countSetFourPieceAvailability(pool)

	// 遍历每个角色，为他们分配最强的可用4件套
	for _, r := range roles {
		bestSet := ""
		bestRank := -1
		
		// 找到最强的可用4件套
		for setName, count := range setAvailability {
			if count <= 0 {
				continue
			}
			
			rank := EquipmentRank[setName]
			if rank > bestRank {
				bestSet = setName
				bestRank = rank
			}
		}
		
		if bestSet == "" {
			allocs = append(allocs, Allocation{RoleName: r.RoleName})
			continue
		}
		
		// 从装备池中取出4件该套装的装备
		items := takePieces(EquipmentSets[bestSet], 4, &pool)
		allocs = append(allocs, Allocation{RoleName: r.RoleName, Items: items})
		
		// 更新套装可用性
		setAvailability[bestSet]--
	}
	
	return allocs
}

func countAvailable(pieces map[string]struct{}, pool []PoolItem) int {
	cnt := 0
	for name := range pieces {
		for _, p := range pool {
			if p.Name == name && p.Count > 0 {
				cnt++
				break
			}
		}
	}
	return cnt
}

func takePieces(pieces map[string]struct{}, need int, pool *[]PoolItem) []string {
	out := []string{}
	for name := range pieces {
		if len(out) >= need {
			break
		}
		for i := range *pool {
			if (*pool)[i].Name == name && (*pool)[i].Count > 0 {
				out = append(out, name)
				(*pool)[i].Count--
				break
			}
		}
	}
	return out
}
