package equipment

import (
	"sort"
	"strings"

	rm "wgserver/internal/services/roles"
	t "wgserver/internal/types"
)

// 标准化位置名到槽位
var slotOrder = []string{"头", "项链", "腰带", "鞋子", "手镯1", "手镯2", "戒指1", "戒指2"}

func normalizeSlot(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "道靴", "鞋子") // 同义
	s = strings.ReplaceAll(s, "法靴", "鞋子")
	if s == "手镯" || s == "手镯1" || s == "手镯2" {
		return s
	}
	if s == "戒指" || s == "戒指1" || s == "戒指2" {
		return s
	}
	// 将中文部位映射
	switch {
	case strings.Contains(s, "头"):
		return "头"
	case strings.Contains(s, "项链"):
		return "项链"
	case strings.Contains(s, "腰带"):
		return "腰带"
	case strings.Contains(s, "靴") || strings.Contains(s, "鞋"):
		return "鞋子"
	case strings.Contains(s, "手镯"):
		return "手镯"
	case strings.Contains(s, "戒指"):
		return "戒指"
	}
	return s
}

type Outfit struct {
	BySlot       map[string]string // 槽位 -> 装备名
	SpecialItems map[string]int    // 特殊物品 -> 数量
}

// 计算一个区服的目标 8 件套（两阶段）
func PlanZone(zone string) map[string]Outfit {
	snap := rm.Instance().SnapshotZone(zone)
	roles := make([]t.RoleAttributes, 0, len(snap.Roles))
	for _, r := range snap.Roles {
		roles = append(roles, r.RoleAttributes)
	}

	// 第一阶段：保障 4 主体，天尊流派优先、道术高优先
	sort.SliceStable(roles, func(i, j int) bool {
		pi, pj := 0, 0
		if roles[i].School == "天尊" {
			pi += 1000
		}
		if roles[j].School == "天尊" {
			pj += 1000
		}
		if roles[i].Magic != roles[j].Magic {
			return roles[i].Magic > roles[j].Magic
		}
		return roles[i].RoleName < roles[j].RoleName
	})

	pool := BuildPool(roles)
	// 先保障四主体
	primary := EnsureFourPiece(roles, pool)
	// 将 primary 合并为 map 以便二阶段填充
	res := map[string]Outfit{}
	for _, a := range primary {
		m := map[string]string{}
		// 粗略放置：根据名字推断槽位
		for _, name := range a.Items {
			placeIntoSlots(m, name)
		}
		res[a.RoleName] = Outfit{BySlot: m, SpecialItems: make(map[string]int)}
	}

	// 第二阶段：按推荐搭配方案补全 8 件，避免相同戒指/手镯重复
	for i := range roles {
		rname := roles[i].RoleName
		of := res[rname]
		fillToEight(&of, roles[i], &pool)
		res[rname] = of
	}
	return res
}

// 将装备名按规则放入空槽位（简化：根据名称含义匹配）
func placeIntoSlots(m map[string]string, name string) {
	slot := guessSlotByName(name)
	if slot == "手镯" {
		if m["手镯1"] == "" {
			m["手镯1"] = name
			return
		}
		if m["手镯2"] == "" {
			m["手镯2"] = name
			return
		}
	}
	if slot == "戒指" {
		if m["戒指1"] == "" {
			m["戒指1"] = name
			return
		}
		if m["戒指2"] == "" {
			m["戒指2"] = name
			return
		}
	}
	if m[slot] == "" {
		m[slot] = name
		return
	}
	// 如果预测槽位占用，则按顺序寻找空位（避免覆盖）
	for _, s := range slotOrder {
		if m[s] == "" {
			m[s] = name
			return
		}
	}
}

func guessSlotByName(name string) string {
	switch {
	case strings.Contains(name, "头盔") || strings.Contains(name, "头"):
		return "头"
	case strings.Contains(name, "项链"):
		return "项链"
	case strings.Contains(name, "腰带"):
		return "腰带"
	case strings.Contains(name, "靴") || strings.Contains(name, "鞋"):
		return "鞋子"
	case strings.Contains(name, "手镯"):
		return "手镯"
	case strings.Contains(name, "戒指"):
		return "戒指"
	default:
		return ""
	}
}

// 组合评分函数，评估组合的强度
func scoreCombo(combo []string, out Outfit, pool []PoolItem, stg Strategy) int {
	score := 0
	
	// 计算每个槽位的装备分数
	for slot, item := range out.BySlot {
		if item == "" {
			continue
		}
		// 查找装备所属的套装
		for setName, pieces := range EquipmentSets {
			if _, exists := pieces[item]; exists {
				// 套装强度加上装备评分
				score += EquipmentRank[setName] * 10
				// 对于主要装备槽位给予额外加分
				if slot == "头" || slot == "项链" || slot == "腰带" || slot == "鞋子" {
					score += 50
				}
				break
			}
		}
	}
	
	// 检查是否有轩辕之心
	if out.SpecialItems[XuanYuanHeart] > 0 {
		score += 1000 // 轩辕之心给予很高的加分
	}
	
	// 检查套装件数（4件套装额外加分）
	for setName, pieces := range EquipmentSets {
		count := 0
		for item := range pieces {
			for _, i := range out.BySlot {
				if i == item {
					count++
					break
				}
			}
		}
		if count >= 4 {
			score += EquipmentRank[setName] * 5 // 4件套装额外加分
		}
	}
	
	return score
}

// 根据职业与推荐方案补全到 8 件
func fillToEight(out *Outfit, ra t.RoleAttributes, pool *[]PoolItem) {
	need := missingSlots(out.BySlot)
	if len(need) == 0 {
		return
	}
	stg := Strategies[ra.Class]
	
	// 查找4件套的套装名称
	var fourPieceSet string
	for _, item := range out.BySlot {
		if item == "" {
			continue
		}
		for setName, items := range EquipmentSets {
			if _, exists := items[item]; exists {
				// 统计该套装在当前装备中的数量
				count := 0
				for _, i := range out.BySlot {
					if i == "" {
						continue
					}
					if _, e := items[i]; e {
						count++
					}
				}
				if count >= 4 {
					fourPieceSet = setName
					break
				}
			}
		if fourPieceSet != "" {
			break
		}
	}

	// 按顺序尝试推荐组合，找到第一个满足条件的就应用
	for _, combo := range stg.Combos {
		outCopy := Outfit{BySlot: cloneMap(out.BySlot), SpecialItems: make(map[string]int)}
		for k, v := range out.SpecialItems {
			outCopy.SpecialItems[k] = v
		}
		poolCopy := clonePool(*pool)
		ok := true
	ComboLoop:
		for _, seg := range combo {
			switch seg {
			case "4主体":
				// 已经尽量满足，不再强行替换
			case "3主体", "2主体", "1主体", "4其他主体", "2其他主体", "3天机", "3天机/疾风", "3疾风", "2祝福":
				if !applySegment(&outCopy, seg, stg, &poolCopy, fourPieceSet) {
					ok = false
					break ComboLoop
				}
			case "1轩辕之心":
				if takeFromPoolExact(&poolCopy, XuanYuanHeart) {
					outCopy.SpecialItems[XuanYuanHeart] = 1
				} else {
					ok = false
					break ComboLoop
				}
			default:
				// 未知段落忽略
			}
		}
		if ok {
			// 校验戒指/手镯不重复名
			if !validNoDuplicatePair(outCopy.BySlot["戒指1"], outCopy.BySlot["戒指2"]) {
				continue
			}
			if !validNoDuplicatePair(outCopy.BySlot["手镯1"], outCopy.BySlot["手镯2"]) {
				continue
			}
			
			// 应用找到的第一个可行组合
			out.BySlot = outCopy.BySlot
			out.SpecialItems = outCopy.SpecialItems
			*pool = poolCopy
			return
		}
	}

	// 若没有组合成功，尝试分配两件套
if len(need) >= 2 {
		// 查找可用的两件套
		for _, setName := range stg.Pri2 {
			pieces := EquipmentSets[setName]
			if pieces == nil {
				continue
			}

			// 检查该套装是否已经被使用（排除4件套）
			if setName != fourPieceSet {
				usedSets := getUsedSets(out, fourPieceSet)
				if usedSets[setName] {
					continue
				}
			}
			
			// 尝试从该套装取2件
			picked := []string{}
			for item := range pieces {
				if len(picked) >= 2 {
					break
				}
				slot := guessSlotByName(item)
				if slot == "" {
					continue
				}
				
				// 检查该槽位是否需要装备
				found := false
				for _, s := range need {
					if s == slot || s == slot+"1" || s == slot+"2" {
						found = true
						break
					}
				}
				if !found {
					continue
				}
				
				// 检查装备是否可用
				if !takeFromPoolExact(pool, item) {
					continue
				}
				
				// 放入合适的槽位
				if slot == "手镯" || slot == "戒指" {
					// 处理双槽位装备
					if out.BySlot[slot+"1"] == "" {
						out.BySlot[slot+"1"] = item
					} else if out.BySlot[slot+"2"] == "" && out.BySlot[slot+"1"] != item {
						out.BySlot[slot+"2"] = item
					} else {
						// 无法放入槽位，将装备放回装备池
						returnToPool(pool, item)
						continue
					}
				} else {
					// 处理单槽位装备
					out.BySlot[slot] = item
				}
				
				picked = append(picked, item)
			}
			
			if len(picked) >= 2 {
				// 成功分配两件套
				return
			}
			
			// 回滚已拿的件
			for _, it := range picked {
				returnToPool(pool, it)
			}
		}
	}

	// 若没有两件套可用，直接用可用的不同件补齐
	for _, s := range slotOrder {
		if out.BySlot[s] == "" {
			// 获取当前已经使用的套装（排除4件套）
			usedSets := getUsedSets(out, fourPieceSet)
			// 找到适合该槽位且不重复套装的装备
			var selectedItem string
			for i := range *pool {
				item := (*pool)[i]
				if item.Count <= 0 {
					continue
				}
				// 检查装备是否适合该槽位
				if !isItemFitting(item.Name, s, out.BySlot) {
					continue
				}
				// 检查装备是否属于已知套装
				isKnown := false
				var itemSet string
				for setName, items := range EquipmentSets {
					if _, exists := items[item.Name]; exists {
						isKnown = true
						itemSet = setName
						break
					}
				}
				// 未知装备不参与分配
				if !isKnown {
					continue
				}
				// 检查套装是否已经被使用（排除4件套）
				if itemSet != fourPieceSet && !usedSets[itemSet] {
					selectedItem = item.Name
					(*pool)[i].Count--
					break
				}
			}
			if selectedItem != "" {
				out.BySlot[s] = selectedItem
			}
		}
	}
}

func missingSlots(m map[string]string) (out []string) {
	for _, s := range slotOrder {
		if m[s] == "" {
			out = append(out, s)
		}
	}
	return
}

func cloneMap(src map[string]string) map[string]string {
	m := make(map[string]string, len(src))
	for k, v := range src {
		m[k] = v
	}
	return m
}

func clonePool(src []PoolItem) []PoolItem {
	cp := make([]PoolItem, len(src))
	copy(cp, src)
	return cp
}

func validNoDuplicatePair(a, b string) bool { return a == "" || b == "" || a != b }

func applySegment(out *Outfit, seg string, stg Strategy, pool *[]PoolItem, fourPieceSet string) bool {
	s := out.BySlot
	// 选择套装来源：主体优先使用 Pri2/Pri1/或指定的“祝福/天机/疾风”等
	srcSets := []string{}
	switch seg {
	case "3主体":
		srcSets = stg.Pri3
	case "2主体":
		srcSets = stg.Pri2
	case "1主体":
		srcSets = stg.Pri1
	case "4其他主体":
		srcSets = allSetsExcept(stg.Pri4)
	case "2其他主体":
		srcSets = allSetsExcept(stg.Pri2)
	case "3天机", "3天机/疾风":
		srcSets = []string{"天机套", "疾风套"}
	case "3疾风":
		srcSets = []string{"疾风套"}
	case "2祝福":
		srcSets = []string{"祝福套"}
	case "1轩辕之心":
		if takeFromPoolExact(pool, XuanYuanHeart) {
			out.SpecialItems[XuanYuanHeart] = 1
			return true
		}
		return false
	default:
		return true
	}
	need := parseCount(seg)
	for _, setName := range srcSets {
		pieces := EquipmentSets[setName]
		if pieces == nil {
			continue
		}

		// 检查该套装是否已经被使用（排除4件套）
		if setName != fourPieceSet {
			usedSets := getUsedSets(out, fourPieceSet)
			if usedSets[setName] {
				continue
			}
		}
		
		// 从该套装尝试取 need 件，且不与现有冲突，并遵守手镯/戒指双件不重复
		picked := []string{}
		for item := range pieces {
			if len(picked) >= need {
				break
			}
			slot := guessSlotByName(item)
			if slot == "手镯" {
				if s["手镯1"] != "" && s["手镯2"] != "" {
					continue
				}
				if containsName(picked, item) {
					continue
				}
				if !takeFromPoolExact(pool, item) {
					continue
				}
				// 放入第一个空的手镯槽，避免与另外一个相同名
				if s["手镯1"] == "" {
					s["手镯1"] = item
				} else if s["手镯2"] == "" && s["手镯1"] != item {
					s["手镯2"] = item
				} else {
					// 无法放入槽位，将装备放回装备池
					returnToPool(pool, item)
					continue
				}
				picked = append(picked, item)
				continue
			}
			if slot == "戒指" {
				if s["戒指1"] != "" && s["戒指2"] != "" {
					continue
				}
				if containsName(picked, item) {
					continue
				}
				if !takeFromPoolExact(pool, item) {
					continue
				}
				if s["戒指1"] == "" {
					s["戒指1"] = item
				} else if s["戒指2"] == "" && s["戒指1"] != item {
					s["戒指2"] = item
				} else {
					// 无法放入槽位，将装备放回装备池
					returnToPool(pool, item)
					continue
				}
				picked = append(picked, item)
				continue
			}
			// 其他槽位
			stdSlot := slot
			if stdSlot == "" {
				stdSlot = firstEmptySlot(s)
			}
			if stdSlot == "" || s[stdSlot] != "" {
				continue
			}
			if !takeFromPoolExact(pool, item) {
				continue
			}
			s[stdSlot] = item
			picked = append(picked, item)
		}
		if len(picked) >= need {
			return true
		}
		// 回滚已拿的件
		for _, it := range picked {
			returnToPool(pool, it)
		}
	}
	return false
}

// 获取当前已经使用的套装（排除指定套装）
func getUsedSets(out *Outfit, excludeSet string) map[string]bool {
	usedSets := make(map[string]bool)
	for _, item := range out.BySlot {
		if item == "" {
			continue
		}
		// 查找装备所属的套装
		for setName, items := range EquipmentSets {
			if _, exists := items[item]; exists {
				if setName != excludeSet {
					usedSets[setName] = true
				}
				break
			}
		}
	}
	return usedSets
}

// 检查装备是否适合某个槽位
func isItemFitting(itemName, slot string, current map[string]string) bool {
	s := guessSlotByName(itemName)
	if slot == "手镯1" || slot == "手镯2" {
		if s != "手镯" {
			return false
		}
	}
	if slot == "戒指1" || slot == "戒指2" {
		if s != "戒指" {
			return false
		}
	}
	if slot == "头" || slot == "项链" || slot == "腰带" || slot == "鞋子" {
		if s != slot {
			return false
		}
	}
	// 避免相同对
	if (slot == "手镯2" && current["手镯1"] == itemName) || (slot == "戒指2" && current["戒指1"] == itemName) {
		return false
	}
	return true
}

func allSetsExcept(ref []string) []string {
	m := map[string]bool{}
	for k := range EquipmentSets {
		m[k] = true
	}
	for _, r := range ref {
		delete(m, r+"套")
		delete(m, r)
	}
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	return out
}

func parseCount(seg string) int {
	if strings.HasPrefix(seg, "4") {
		return 4
	}
	if strings.HasPrefix(seg, "3") {
		return 3
	}
	if strings.HasPrefix(seg, "2") {
		return 2
	}
	if strings.HasPrefix(seg, "1") {
		return 1
	}
	return 0
}

func firstEmptySlot(s map[string]string) string {
	for _, sl := range slotOrder {
		if s[sl] == "" {
			return sl
		}
	}
	return ""
}

func containsName(list []string, name string) bool {
	for _, v := range list {
		if v == name {
			return true
		}
	}
	return false
}

func takeFromPoolExact(pool *[]PoolItem, name string) bool {
	for i := range *pool {
		if (*pool)[i].Name == name && (*pool)[i].Count > 0 {
			(*pool)[i].Count--
			return true
		}
	}
	return false
}

func returnToPool(pool *[]PoolItem, name string) {
	for i := range *pool {
		if (*pool)[i].Name == name {
			(*pool)[i].Count++
			return
		}
	}
	// 不在池中则新增（罕见）
	*pool = append(*pool, PoolItem{Name: name, Count: 1})
}

func takeAnyFitting(pool *[]PoolItem, slot string, current map[string]string) string {
	for i := range *pool {
		p := &(*pool)[i]
		if p.Count <= 0 {
			continue
		}
		s := guessSlotByName(p.Name)
		if slot == "手镯1" || slot == "手镯2" {
			if s != "手镯" {
				continue
			}
		}
		if slot == "戒指1" || slot == "戒指2" {
			if s != "戒指" {
				continue
			}
		}
		if slot == "头" || slot == "项链" || slot == "腰带" || slot == "鞋子" {
			if s != slot {
				continue
			}
		}
		// 避免相同对
		if (slot == "手镯2" && current["手镯1"] == p.Name) || (slot == "戒指2" && current["戒指1"] == p.Name) {
			continue
		}
		p.Count--
		return p.Name
	}
	return ""
}
