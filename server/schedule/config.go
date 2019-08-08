package schedule

type Config interface {
	LoadConfig() bool
	GetStoreId(cluster Cluster) map[string][]uint64
	GetRegionId(cluster Cluster) map[string][]uint64
	GetInterval() map[string]*TimeInterval
	IfConflict() bool
	IfNeedCheckStore() [][]int
}
