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

// Greedy: ensure each role gets a 4-piece main set if possible
func EnsureFourPiece(roles []t.RoleAttributes, pool []PoolItem) []Allocation {
	// order roles: priority 天尊流派其次道术
	sort.SliceStable(roles, func(i, j int) bool {
		pi := 0
		pj := 0
		if roles[i].School == "天尊" {
			pi += 1000
		}
		if roles[j].School == "天尊" {
			pj += 1000
		}
		if roles[i].Magic != roles[j].Magic {
			return roles[i].Magic > roles[j].Magic
		}
		return pi > pj
	})

	allocs := make([]Allocation, 0, len(roles))
	// very simplified matching: find best-ranked set with >=4 distinct slots available
	for _, r := range roles {
		bestSet := ""
		bestRank := -1
		for set, pieces := range EquipmentSets {
			if rank := EquipmentRank[set]; rank > bestRank {
				if countAvailable(pieces, pool) >= 4 {
					bestSet = set
					bestRank = rank
				}
			}
		}
		if bestSet == "" {
			allocs = append(allocs, Allocation{RoleName: r.RoleName})
			continue
		}
		// take 4 distinct piece names
		items := takePieces(EquipmentSets[bestSet], 4, &pool)
		allocs = append(allocs, Allocation{RoleName: r.RoleName, Items: items})
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
