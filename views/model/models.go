package model

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