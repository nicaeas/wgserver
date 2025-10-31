package alloc

import (
	"sort"
	"strings"

	"wgserver/internal/logger"
	eq "wgserver/internal/services/equipment"
	"wgserver/internal/services/roles"
)

type MapTarget struct {
	Map   string
	Floor int
}

type Assignment struct {
	RoleName string
	Target   MapTarget
}

// Difficulty order low->high; we will create reversed for high-first needs
var mapsOrder = []string{"将军坟", "将军坟东", "机关洞", "五蛇殿", "玄冰古道", "远古机关洞", "远古蛇殿", "通天塔", "禁地魔穴", "远古逆魔", "地下魔域"}

// Minimal counts per merge state. If insufficient roles, apply fallback as spec.

func requiredCounts(merge string) (total int, mageFixed []MapTarget, otherPlan map[string]int) {
	switch merge {
	case "未合区":
		// 2 mages fixed to 机关洞5/6层
		mageFixed = []MapTarget{{"机关洞", 5}, {"机关洞", 6}}
		// base other 10 as starting state (case 1) - dynamic adjustment later by eq
		otherPlan = map[string]int{"将军坟": 1, "将军坟东": 1, "机关洞": 3, "五蛇殿": 5}
		total = 12
	case "一合", "一合区", "第一次合区":
		mageFixed = []MapTarget{{"机关洞", 5}, {"机关洞", 6}, {"远古机关洞", 2}, {"远古机关洞", 3}}
		otherPlan = map[string]int{} // dynamic within function 2 rules
		total = 24
	case "二合", "三合", "四合", "五合", "六合":
		mageFixed = []MapTarget{{"机关洞", 5}, {"机关洞", 6}, {"玄冰古道", 2}, {"玄冰古道", 3}, {"远古机关洞", 2}, {"远古机关洞", 3}, {"远古逆魔", 1}, {"远古逆魔", 2}}
		otherPlan = map[string]int{"将军坟": 2, "机关洞": 2, "五蛇殿": 2, "远古机关洞": 2, "远古蛇殿": 3, "通天塔": 6, "禁地魔穴": 6, "远古逆魔": 9, "地下魔域": 8}
		total = 48
	case "七合", "七合以后":
		mageFixed = []MapTarget{{"地下魔域", 1}, {"地下魔域", 2}}
		otherPlan = map[string]int{"将军坟": 1, "机关洞": 1, "五蛇殿": 1, "远古机关洞": 1, "远古蛇殿": 1, "通天塔": 4, "禁地魔穴": 5, "远古逆魔": 6, "地下魔域": 6}
		total = 28
	default:
		// default treat as 未合区
		mageFixed = []MapTarget{{"机关洞", 5}, {"机关洞", 6}}
		otherPlan = map[string]int{"将军坟": 1, "将军坟东": 1, "机关洞": 3, "五蛇殿": 5}
		total = 12
	}
	return
}

func isMage(class string) bool { return class == "法师" }

func luckyHigh(lucky int) bool { return lucky >= 9 }

func strengthScore(r *roles.RoleInfo) int {
	base := r.Magic
	if luckyHigh(r.Lucky) {
		base += 10000
	}
	if r.Level >= 60 {
		base += 5000
	}
	return base
}

// ---------- 天尊或以上(>=A) 4主体判定（基于当前已穿戴装备信息） ----------
func hasATierFourPiece(r *roles.RoleInfo) bool {
	// 反向索引：装备名 -> 套装名
	// 遍历已穿戴：按部位去重（戒指/手镯同名不可算两件）
	// 判定某套装不同部位数量>=4 且该套强度>=A(>=70)
	// 构建逆索引一次（简单起见每次构建）
	itemToSet := map[string]string{}
	for setName, items := range eq.EquipmentSets {
		for name := range items {
			itemToSet[name] = setName
		}
	}
	// 同套不同部位计数
	type uniq struct {
		pos  string
		name string
	}
	setPos := map[string]map[string]struct{}{}
	for _, e := range r.Equipments {
		setName, ok := itemToSet[e.Name]
		if !ok {
			continue
		}
		rank := eq.EquipmentRank[setName]
		if rank < 70 {
			continue
		}
		pos := normalizePos(e.Slot)
		if pos == "手镯" {
			pos = "手镯"
		} // 两个手镯视为同类位置，不重复计数
		if pos == "戒指" {
			pos = "戒指"
		}
		if setPos[setName] == nil {
			setPos[setName] = map[string]struct{}{}
		}
		// 去重：相同位置只计一次
		setPos[setName][pos] = struct{}{}
	}
	for setName, posSet := range setPos {
		if len(posSet) >= 4 && eq.EquipmentRank[setName] >= 70 {
			return true
		}
	}
	return false
}

func normalizePos(p string) string {
	if p == "手镯1" || p == "手镯2" {
		return "手镯"
	}
	if p == "戒指1" || p == "戒指2" {
		return "戒指"
	}
	switch {
	case strings.Contains(p, "头"):
		return "头"
	case strings.Contains(p, "项链"):
		return "项链"
	case strings.Contains(p, "腰带"):
		return "腰带"
	case strings.Contains(p, "靴") || strings.Contains(p, "鞋"):
		return "鞋子"
	case strings.Contains(p, "手镯"):
		return "手镯"
	case strings.Contains(p, "戒指"):
		return "戒指"
	default:
		return p
	}
}

// Evaluate and push assignments for a zone
func Plan(zone string) []Assignment {
	m := roles.Instance()
	zs := m.SnapshotZone(zone)
	if len(zs.Roles) == 0 {
		return nil
	}

	// Group by mage/others
	var mages, others []*roles.RoleInfo
	for _, r := range zs.Roles {
		if isMage(r.Class) {
			mages = append(mages, r)
		} else {
			others = append(others, r)
		}
	}

	// Sort by strength desc for others and mages
	sort.Slice(others, func(i, j int) bool { return strengthScore(others[i]) > strengthScore(others[j]) })
	sort.Slice(mages, func(i, j int) bool { return strengthScore(mages[i]) > strengthScore(mages[j]) })

	// Determine required counts
	totalRequired, mageFixed, otherPlan := requiredCounts(zsAnyMerge(zs))
	insufficient := len(zs.Roles) < totalRequired

	as := []Assignment{}
	// Assign mages: 若人数不足，按高难顺序分配；否则按固定表
	mageTargets := mageFixed
	if insufficient {
		mageTargets = mageInsufficientOrder(zsAnyMerge(zs))
		if len(mageTargets) == 0 {
			mageTargets = mageFixed
		}
	}
	assignMageN := min(len(mages), len(mageTargets))
	for i := 0; i < assignMageN; i++ {
		as = append(as, Assignment{RoleName: mages[i].RoleName, Target: mageTargets[i]})
	}
	// 多余法师并入其他职业流程
	if len(mages) > assignMageN {
		others = append(mages[assignMageN:], others...)
	}

	// 人数不足：统一执行“覆盖+填充”的通用逻辑；否则按各合区完整规则
	if insufficient {
		as = append(as, assignOthers_Insufficient(others, otherPlan, zsAnyMerge(zs))...)
	} else {
		// Apply specific rules for 未合区与一合
		if strings.Contains(zsAnyMerge(zs), "未合") {
			as = append(as, assignOthers_Unmerged(others, otherPlan)...)
		} else if strings.Contains(zsAnyMerge(zs), "一合") || strings.Contains(zsAnyMerge(zs), "第一次合区") {
			as = append(as, assignOthers_Merge1(others)...)
		} else {
			as = append(as, assignOthers_FixedPlan(others, otherPlan)...)
		}
	}
	logger.MapAlloc().Printf("zone=%s plan assignments=%d", zone, len(as))
	return as
}

func zsAnyMerge(zs *roles.ZoneState) string {
	for _, r := range zs.Roles {
		return r.MergeState
	}
	return "未合区"
}

// 不足人数时：其他职业“覆盖+填充”
func assignOthers_Insufficient(others []*roles.RoleInfo, plan map[string]int, merge string) []Assignment {
	if len(others) == 0 {
		return nil
	}
	// Lucky9优先，其次强度
	sort.SliceStable(others, func(i, j int) bool {
		li, lj := luckyHigh(others[i].Lucky), luckyHigh(others[j].Lucky)
		if li != lj {
			return li
		}
		return strengthScore(others[i]) > strengthScore(others[j])
	})
	// 地图优先级（高->低）
	order := otherMapsOrderHigh(merge)
	// 若提供了计划数量，仅保留在计划中的图；否则全部保留
	filtered := []string{}
	if len(plan) > 0 {
		for _, m := range order {
			if _, ok := plan[m]; ok {
				filtered = append(filtered, m)
			}
		}
	} else {
		filtered = order
	}
	// 拷贝一份可写计数；plan 为空时用一个大数代表“充足”
	counts := map[string]int{}
	if len(plan) > 0 {
		for k, v := range plan {
			counts[k] = v
		}
	} else {
		for _, m := range filtered {
			counts[m] = 1 << 30
		}
	}
	used := map[string]bool{}
	res := []Assignment{}
	// Step1: 覆盖——每张规划图至少放1人（若有可进入者）
	for _, m := range filtered {
		if counts[m] <= 0 {
			continue
		}
		// 选择最优可进入的候选
		for _, r := range others {
			if used[r.RoleName] {
				continue
			}
			if (m == "远古逆魔" || m == "地下魔域") && r.Level < 60 {
				continue
			}
			res = append(res, Assignment{RoleName: r.RoleName, Target: MapTarget{Map: m, Floor: 1}})
			used[r.RoleName] = true
			counts[m]--
			break
		}
	}
	// Step2: 填充——从最高难开始，直至该图数量满足，再到下一图
	for _, m := range filtered {
		for counts[m] > 0 {
			picked := false
			for _, r := range others {
				if used[r.RoleName] {
					continue
				}
				if (m == "远古逆魔" || m == "地下魔域") && r.Level < 60 {
					continue
				}
				res = append(res, Assignment{RoleName: r.RoleName, Target: MapTarget{Map: m, Floor: 1}})
				used[r.RoleName] = true
				counts[m]--
				picked = true
				break
			}
			if !picked {
				break
			} // 没有更多可用人选
		}
	}
	// 若仍有未用完的角色（无配额限制或所有配额未填完但都不适配），按次高难溢出
	for _, r := range others {
		if used[r.RoleName] {
			continue
		}
		// 找一个能进的最高难地图
		for _, m := range filtered {
			if (m == "远古逆魔" || m == "地下魔域") && r.Level < 60 {
				continue
			}
			res = append(res, Assignment{RoleName: r.RoleName, Target: MapTarget{Map: m, Floor: 1}})
			used[r.RoleName] = true
			break
		}
	}
	return res
}

func otherMapsOrderHigh(merge string) []string {
	switch {
	case strings.Contains(merge, "七合"):
		return []string{"地下魔域", "远古逆魔", "禁地魔穴", "通天塔", "远古蛇殿", "远古机关洞", "五蛇殿", "机关洞", "将军坟"}
	case strings.Contains(merge, "二合") || strings.Contains(merge, "三合") || strings.Contains(merge, "四合") || strings.Contains(merge, "五合") || strings.Contains(merge, "六合"):
		return []string{"地下魔域", "远古逆魔", "禁地魔穴", "通天塔", "远古蛇殿", "远古机关洞", "玄冰古道", "五蛇殿", "机关洞", "将军坟"}
	case strings.Contains(merge, "一合"):
		return []string{"通天塔", "禁地魔穴", "远古蛇殿", "五蛇殿", "机关洞", "将军坟"}
	default: // 未合区
		return []string{"五蛇殿", "机关洞", "将军坟东", "将军坟"}
	}
}

// 不足人数时：法师目标按高难度排序
func mageInsufficientOrder(merge string) []MapTarget {
	switch {
	case strings.Contains(merge, "七合"):
		return []MapTarget{{Map: "地下魔域", Floor: 2}, {Map: "地下魔域", Floor: 1}}
	case strings.Contains(merge, "二合") || strings.Contains(merge, "三合") || strings.Contains(merge, "四合") || strings.Contains(merge, "五合") || strings.Contains(merge, "六合"):
		return []MapTarget{{Map: "远古逆魔", Floor: 2}, {Map: "远古逆魔", Floor: 1}, {Map: "远古机关洞", Floor: 3}, {Map: "远古机关洞", Floor: 2}, {Map: "玄冰古道", Floor: 3}, {Map: "玄冰古道", Floor: 2}, {Map: "机关洞", Floor: 6}, {Map: "机关洞", Floor: 5}}
	case strings.Contains(merge, "一合"):
		return []MapTarget{{Map: "远古机关洞", Floor: 3}, {Map: "远古机关洞", Floor: 2}, {Map: "机关洞", Floor: 6}, {Map: "机关洞", Floor: 5}}
	default: // 未合区
		return []MapTarget{{Map: "机关洞", Floor: 6}, {Map: "机关洞", Floor: 5}}
	}
}

// 未合区其他职业分配：完整实现 1)~5) 以及 6) 技能150细则
func assignOthers_Unmerged(others []*roles.RoleInfo, basePlan map[string]int) []Assignment {
	if len(others) == 0 {
		return nil
	}
	// 标记具备 A 级及以上四主体（天尊或更高）的角色
	var eligible, rest []*roles.RoleInfo
	for _, r := range others {
		if hasATierFourPiece(r) {
			eligible = append(eligible, r)
		} else {
			rest = append(rest, r)
		}
	}
	sort.SliceStable(eligible, func(i, j int) bool { return strengthScore(eligible[i]) > strengthScore(eligible[j]) })
	sort.SliceStable(rest, func(i, j int) bool { return strengthScore(rest[i]) > strengthScore(rest[j]) })

	// 统计技能150人数（仅在 eligible 条件满足 4 个后才应用第6步）
	skill150 := 0
	for _, r := range others {
		if r.Skill >= 150 {
			skill150++
		}
	}

	plan := map[string]int{}
	// 先处理 1)~5)
	switch x := min(len(eligible), 4); x {
	case 0:
		for k, v := range basePlan {
			plan[k] = v
		}
	case 1:
		plan = map[string]int{"将军坟": 1, "将军坟东": 1, "机关洞": 3, "五蛇殿": 4, "通天塔": 1}
	case 2:
		plan = map[string]int{"将军坟": 1, "将军坟东": 1, "机关洞": 2, "五蛇殿": 4, "通天塔": 2}
	case 3:
		plan = map[string]int{"将军坟": 1, "将军坟东": 1, "机关洞": 2, "五蛇殿": 3, "通天塔": 3}
	case 4:
		plan = map[string]int{"将军坟": 1, "机关洞": 2, "五蛇殿": 3, "通天塔": 4}
	}
	// 第6步：当达到4个 eligible 后，按已达成的 skill150 数量动态调整
	if len(eligible) >= 4 {
		switch {
		case skill150 >= 1 && skill150 < 2:
			plan = map[string]int{"将军坟": 1, "机关洞": 2, "五蛇殿": 2, "通天塔": 4, "禁地魔穴": 1}
		case skill150 == 2:
			plan = map[string]int{"将军坟": 1, "机关洞": 1, "五蛇殿": 2, "通天塔": 4, "禁地魔穴": 2}
		case skill150 == 3:
			plan = map[string]int{"将军坟": 1, "机关洞": 1, "五蛇殿": 1, "通天塔": 4, "禁地魔穴": 3}
		case skill150 == 4:
			// 第4个技能150进入通天塔（但TT最多4）
			plan = map[string]int{"将军坟": 1, "机关洞": 1, "五蛇殿": 1, "通天塔": 4, "禁地魔穴": 3}
		case skill150 == 5 || skill150 == 6 || skill150 == 7:
			plan = map[string]int{"将军坟": 1, "机关洞": 1, "五蛇殿": 1, "通天塔": 4, "禁地魔穴": 3}
		case skill150 >= 8:
			plan = map[string]int{"机关洞": 1, "五蛇殿": 1, "通天塔": 4, "禁地魔穴": 3, "远古蛇殿": 1}
		}
	}
	// 分配：优先将通天塔数量分给 eligible（幸运9和高道术优先）
	sort.SliceStable(others, func(i, j int) bool {
		li, lj := luckyHigh(others[i].Lucky), luckyHigh(others[j].Lucky)
		if li != lj {
			return li
		}
		return strengthScore(others[i]) > strengthScore(others[j])
	})
	used := map[string]bool{}
	result := []Assignment{}
	// 先通天塔
	ttNeed := plan["通天塔"]
	if ttNeed > 0 {
		for _, r := range eligible {
			if ttNeed == 0 {
				break
			}
			if used[r.RoleName] {
				continue
			}
			result = append(result, Assignment{RoleName: r.RoleName, Target: MapTarget{Map: "通天塔", Floor: 1}})
			used[r.RoleName] = true
			ttNeed--
		}
		plan["通天塔"] = ttNeed
	}
	// 再禁地魔穴（若有）
	jindi := plan["禁地魔穴"]
	for _, r := range others {
		if jindi == 0 {
			break
		}
		if used[r.RoleName] {
			continue
		}
		result = append(result, Assignment{RoleName: r.RoleName, Target: MapTarget{Map: "禁地魔穴", Floor: 1}})
		used[r.RoleName] = true
		jindi--
	}
	plan["禁地魔穴"] = jindi
	// 远古蛇殿（若有）
	fgsd := plan["远古蛇殿"]
	for _, r := range others {
		if fgsd == 0 {
			break
		}
		if used[r.RoleName] {
			continue
		}
		result = append(result, Assignment{RoleName: r.RoleName, Target: MapTarget{Map: "远古蛇殿", Floor: 1}})
		used[r.RoleName] = true
		fgsd--
	}
	plan["远古蛇殿"] = fgsd
	// 其余地图按难度从高到低/幸运9优先填充
	order := []string{"五蛇殿", "机关洞", "将军坟东", "将军坟"}
	for _, mname := range order {
		need := plan[mname]
		for _, r := range others {
			if need == 0 {
				break
			}
			if used[r.RoleName] {
				continue
			}
			result = append(result, Assignment{RoleName: r.RoleName, Target: MapTarget{Map: mname, Floor: 1}})
			used[r.RoleName] = true
			need--
		}
		plan[mname] = need
	}
	// 如果仍有未分配的通天塔名额（eligible 不够），用剩余强者填充
	if plan["通天塔"] > 0 {
		need := plan["通天塔"]
		for _, r := range others {
			if need == 0 {
				break
			}
			if used[r.RoleName] {
				continue
			}
			result = append(result, Assignment{RoleName: r.RoleName, Target: MapTarget{Map: "通天塔", Floor: 1}})
			used[r.RoleName] = true
			need--
		}
		plan["通天塔"] = need
	}
	// 若还有未分配（理论上不会），溢出到五蛇殿
	for _, r := range others {
		if used[r.RoleName] {
			continue
		}
		result = append(result, Assignment{RoleName: r.RoleName, Target: MapTarget{Map: "五蛇殿", Floor: 1}})
		used[r.RoleName] = true
	}
	return result
}

// 一合：按规则将最低强度5人分配：机关洞1、五蛇殿2、远古蛇殿2；其余15人动态：通天塔/禁地魔穴与60级后的远古逆魔、地下魔域
func assignOthers_Merge1(others []*roles.RoleInfo) []Assignment {
	assign := []Assignment{}
	if len(others) == 0 {
		return assign
	}
	low := min(5, len(others))
	// lowest 5
	last := len(others)
	lo := others[last-low:]
	base := map[string]int{"机关洞": 1, "五蛇殿": 2, "远古蛇殿": 2}
	assign = append(assign, distributeByNeed(lo, base)...)
	// remaining
	rest := others[:last-low]
	// initial 8 TT, 7 禁地
	counts := map[string]int{"通天塔": 8, "禁地魔穴": 7}
	// count of >=60
	var sixty []*roles.RoleInfo
	for _, r := range rest {
		if r.Level >= 60 {
			sixty = append(sixty, r)
		}
	}
	sort.Slice(sixty, func(i, j int) bool { return strengthScore(sixty[i]) > strengthScore(sixty[j]) })
	n := len(sixty)
	switch {
	case n >= 7:
		assign = append(assign, topTo(sixty, 4, "地下魔域")...)
		assign = append(assign, nextTo(sixty, 3, 4, "远古逆魔")...)
		counts = map[string]int{"通天塔": 4, "禁地魔穴": 4}
	case n == 6:
		assign = append(assign, topTo(sixty, 3, "地下魔域")...)
		assign = append(assign, nextTo(sixty, 3, 3, "远古逆魔")...)
		counts = map[string]int{"通天塔": 5, "禁地魔穴": 4}
	case n == 5:
		assign = append(assign, topTo(sixty, 2, "地下魔域")...)
		assign = append(assign, nextTo(sixty, 3, 2, "远古逆魔")...)
		counts = map[string]int{"通天塔": 5, "禁地魔穴": 5}
	case n == 4:
		assign = append(assign, topTo(sixty, 1, "地下魔域")...)
		assign = append(assign, nextTo(sixty, 3, 1, "远古逆魔")...)
		counts = map[string]int{"通天塔": 6, "禁地魔穴": 5}
	case n == 3:
		assign = append(assign, topTo(sixty, 0, "地下魔域")...)
		assign = append(assign, nextTo(sixty, 3, 0, "远古逆魔")...)
		counts = map[string]int{"通天塔": 6, "禁地魔穴": 6}
	case n == 2:
		assign = append(assign, topTo(sixty, 0, "地下魔域")...)
		assign = append(assign, nextTo(sixty, 2, 0, "远古逆魔")...)
		counts = map[string]int{"通天塔": 7, "禁地魔穴": 6}
	case n == 1:
		assign = append(assign, topTo(sixty, 0, "地下魔域")...)
		assign = append(assign, nextTo(sixty, 1, 0, "远古逆魔")...)
		counts = map[string]int{"通天塔": 7, "禁地魔穴": 7}
	default:
		// no 60
	}
	assign = append(assign, distributeByNeed(rest, counts)...)
	return assign
}

// 二-六合与七合以后：直接按固定人数计划，优先幸运9与高道术
func assignOthers_FixedPlan(others []*roles.RoleInfo, plan map[string]int) []Assignment {
	return distributeByNeed(others, plan)
}

// 通用分配：按幸运9优先/强度高优先，逐个角色为其选择能进入且仍有名额的最高难度地图
func distributeByNeed(others []*roles.RoleInfo, plan map[string]int) []Assignment {
	highToLow := []string{"地下魔域", "远古逆魔", "禁地魔穴", "通天塔", "远古蛇殿", "远古机关洞", "玄冰古道", "五蛇殿", "机关洞", "将军坟东", "将军坟"}
	sort.SliceStable(others, func(i, j int) bool {
		li, lj := luckyHigh(others[i].Lucky), luckyHigh(others[j].Lucky)
		if li != lj {
			return li
		}
		return strengthScore(others[i]) > strengthScore(others[j])
	})
	out := []Assignment{}
	for _, r := range others {
		for _, mp := range highToLow {
			need := plan[mp]
			if need <= 0 {
				continue
			}
			// 60级门槛
			if (mp == "远古逆魔" || mp == "地下魔域") && r.Level < 60 {
				continue
			}
			out = append(out, Assignment{RoleName: r.RoleName, Target: MapTarget{Map: mp, Floor: 1}})
			plan[mp] = need - 1
			break
		}
	}
	return out
}

func topTo(arr []*roles.RoleInfo, take int, m string) []Assignment {
	assign := []Assignment{}
	if take <= 0 {
		return assign
	}
	for i := 0; i < take && i < len(arr); i++ {
		assign = append(assign, Assignment{RoleName: arr[i].RoleName, Target: MapTarget{Map: m, Floor: 1}})
	}
	return assign
}

func nextTo(arr []*roles.RoleInfo, take int, offset int, m string) []Assignment {
	assign := []Assignment{}
	for i := offset; i < offset+take && i < len(arr); i++ {
		assign = append(assign, Assignment{RoleName: arr[i].RoleName, Target: MapTarget{Map: m, Floor: 1}})
	}
	return assign
}

// no scheduler here to avoid import cycles; server package calls Plan and pushes
