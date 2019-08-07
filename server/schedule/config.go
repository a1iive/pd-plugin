package schedule

type Config interface {
	LoadConfig()
	GetStoreId(cluster Cluster) map[string][]uint64
	GetRegionId(cluster Cluster) map[string][]uint64
	GetInterval() map[string]*TimeInterval
}
