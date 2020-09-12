package model

type NodeModel struct {
	UID,
	Name,
	Status,
	Role,
	Version,
	CpuUsage,
	CpuAvail string
	CpuValue,
	CpuAvailValue int64
	MemUsage,
	MemAvail string
	MemValue,
	MemAvailValue int64
}
