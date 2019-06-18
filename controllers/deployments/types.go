package deployments

type usage struct {
	cpuUsage,
	cpuAvail,
	memUsage,
	memAvail int64
}

type deployRow struct {
	name string
}
