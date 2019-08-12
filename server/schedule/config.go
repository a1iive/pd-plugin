package schedule

type Config interface {
	LoadConfig(path string) bool
	GetStoreId(cluster Cluster) map[string][]uint64
	GetInterval() map[string]*TimeInterval
	IfConflict() bool
	IfNeedCheckStore() [][]int
}
