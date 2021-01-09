package model

type NodeModel struct {
	UID,
	Name,
	InternalIp,
	ExternalIp,
	Hostname,
	Status,
	Role,
	Version,
	OS,
	OSImage,
	OSKernel,
	Architecture string
	CpuUsage string
	CpuAvail,
	CpuValue,
	CpuAvailValue int64
	MemUsage string
	MemAvail,
	MemValue,
	MemAvailValue,
	StorageAvail int64
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