# wgserver 游戏角色规划服务器

功能概述：
- WebSocket 长连接（端口 8888），客户端连接后分配唯一 client_id 并下发 connection_ack
- 应用级心跳：服务端每 30 秒发送 `{type:"heartbeat"}`；3 分钟未收到心跳回复则断开并清理对应客户端角色
- 角色属性接收与区服聚合：同一充值区服作为统一规划域
- 副本地图分配：按合服状态、职业、道术、幸运等策略规划，并每 30 秒推送一次分配结果
- 日常任务队列：同区最多 3 个并发，先来先服务
- 设备分配（骨架）：统一资源池与四主体优先保证，后续可扩展为完整推荐策略
- MySQL 数据库：初始化脚本见 db/schema.sql
- 分类型按日日志：logs/log_YYYYMMDD_*.log

## 运行

1. 配置 MySQL（可选环境变量）
- MYSQL_DSN（默认：`root:password@tcp(127.0.0.1:3306)/wgserver?parseTime=true&charset=utf8mb4,utf8`）
- PORT（默认：8888）

2. 初始化数据库
- 执行 `db/schema.sql`

3. 构建与运行（Windows PowerShell）
```powershell
# 在项目根目录
go mod tidy
go build -o bin\wgserver.exe ./cmd/server
./bin/wgserver.exe
```

4. WebSocket 连接
- 路径：`ws://127.0.0.1:8888/ws`
- 首帧服务端返回：
```json
{"code":200,"Message":"成功","type":"connection_ack","client_id":"..."}
```

5. 心跳
- 服务端每 30s 发送：`{"type":"heartbeat","client_id":"..."}`
- 客户端需回复：`{"type":"heartbeat_response","client_id":"...","status":"alive"}`

6. 非心跳消息 ACK
- 客户端收到任何非心跳消息后应回复：`{"client_id":"...","status":"received"}`

7. 角色属性上报
- 按需求文档 JSON 字段发送；服务端达到人数阈值或等待超时将执行分配并每 30s 推送：
```json
{"角色名":"小小鸟","data":{"地图":"远古机关洞","层数":1},"client_id":"..."}
```

8. 日常任务队列
- 开始：
```json
{"角色名":"A","合区区服":"中州1区","消息类型":"日常任务","任务状态":"开始","client_id":"..."}
```
- 服务端回复：`任务状态` 为 `允许` 或 `等待`
- 完成：
```json
{"角色名":"A","合区区服":"中州1区","消息类型":"日常任务","任务状态":"完成","client_id":"..."}
```

## 目录结构
- cmd/server/main.go 启动入口
- internal/config 配置
- internal/logger 日志
- internal/db 数据库连接
- internal/server WebSocket Hub/协议处理
- internal/services/roles 区服/角色管理
- internal/services/alloc 副本分配逻辑
- internal/services/tasks 日常任务队列
- internal/services/equipment 装备分配（骨架）
- db/schema.sql 初始化 SQL

## 注意
- 目前装备分配与交换事务提供了骨架与数据结构，后续迭代可按策略补全发送“分配指令/接收指令/坐标确认/结果确认”的原子流程。
- 副本分配规则已覆盖未合区/一合（含60级升级路径）和二-六合/七合以后固定方案，未合区技能150的细化升级策略可在后续迭代中补充。
- 日志按变更/事件写入，避免重复膨胀。
