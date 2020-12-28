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

type PodModel struct {
	UID,
	Namespace,
	Name,
	Status,
	Node,
	IP string
	PodCPUValue,
	PodMemValue,
	NodeCPUValue,
	NodeMemValue int64
	Volumes int
}