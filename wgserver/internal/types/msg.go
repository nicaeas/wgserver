package types

type MsgType string

const (
	MsgTypeHeartbeatResponse MsgType = "heartbeat_response"
	MsgTypeHeartbeat         MsgType = "heartbeat"
	MsgTypeConnectionAck     MsgType = "connection_ack"
	MsgTypeDailyTask         MsgType = "日常任务"
)
