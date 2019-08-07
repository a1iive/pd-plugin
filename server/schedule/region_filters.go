package schedule

import (
	"github.com/pingcap/pd/server/core"
)

type RegionFilter interface {
	Type() string
	FilterSource(opt Options, region *core.RegionInfo, interval *TimeInterval, regionIDs []uint64) bool
	FilterTarget(opt Options, region *core.RegionInfo, interval *TimeInterval, regionIDs []uint64) bool
}

// FilterSource checks if region can pass all Filters as source region.
func RegionFilterSource(opt Options, region *core.RegionInfo, filters []RegionFilter, interval *TimeInterval, regionIDs []uint64) bool {
	for _, filter := range filters {
		if filter.FilterSource(opt, region, interval, regionIDs) {
			return true
		}
	}
	return false
}
