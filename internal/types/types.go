package types

// Shared types for messages and models

type DailyTaskMessage struct {
	RoleName   string `json:"角色名"`
	Zone       string `json:"充值区服"`
	MsgType    string `json:"消息类型"`
	TaskStatus string `json:"任务状态"`
	ClientID   string `json:"client_id"`
}

type RoleAttributes struct {
	MapName    string      `json:"当前所在地图" db:"current_map"`
	RoleName   string      `json:"角色名" db:"role_name"`
	Zone       string      `json:"充值区服" db:"zone"`
	MergeState string      `json:"合区" db:"merge_state"`
	Class      string      `json:"职业" db:"class"`
	School     string      `json:"流派" db:"school"`
	Skill      int         `json:"技能" db:"skill"`
	Level      int         `json:"等级" db:"level"`
	Lucky      int         `json:"幸运" db:"lucky"`
	Magic      int         `json:"道术" db:"magic"`
	Gold       int         `json:"金币" db:"gold"`
	Yuanbao    int         `json:"元宝" db:"yuanbao"`
	HP         int         `json:"血量" db:"hp"`
	ClientID   string      `json:"client_id" db:"client_id"`
	CreatedAt  string      `json:"创角时间" db:"created_at"`
	X          int         `json:"X" db:"x"`
	Y          int         `json:"Y" db:"y"`
	Equipments []EquipItem `json:"装备信息"`
	Backpack   []Item      `json:"背包信息"`
	Warehouse  []Item      `json:"仓库信息"`
}

type EquipItem struct {
	Slot string `json:"部位"`
	Name string `json:"装备名"`
}

type Item struct {
	Name    string `json:"物品名字"`
	Count   int    `json:"物品数量"`
	ItemLvl int    `json:"物品等级"`
	Enhance int    `json:"强化等级"`
	Refine  int    `json:"淬炼等级"`
}
