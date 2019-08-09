package main

import (
	"github.com/pingcap/pd/server/core"
	"github.com/pingcap/pd/server/schedule"
	"time"
)

type violentFilter struct {
}

func NewViolentFilter() schedule.RegionFilter {
	return &violentFilter{}
}

func (f *violentFilter) Type() string {
	return "leader-filter"
}

func (f *violentFilter) FilterSource(opt schedule.Options, region *core.RegionInfo, interval *schedule.TimeInterval, regionIDs []uint64) bool {
	if interval != nil {
		currentTime := time.Now()
		if currentTime.After(interval.End) || interval.Begin.After(currentTime) {
			return false
		}
	}
	return f.isExists(region.GetID(), regionIDs)
}

func (f *violentFilter) FilterTarget(opt schedule.Options, region *core.RegionInfo, interval *schedule.TimeInterval, regionIDs []uint64) bool {
	if interval != nil {
		currentTime := time.Now()
		if currentTime.After(interval.End) || interval.Begin.After(currentTime) {
			return false
		}
	}
	return f.isExists(region.GetID(), regionIDs)
}

func (f *violentFilter) isExists(regionID uint64, regionIDs []uint64) bool {
	for _, id := range regionIDs {
		if id == regionID {
			return true
		}
	}
	return false
}
