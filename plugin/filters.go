package main

import (
	"github.com/pingcap/pd/server/core"
	"github.com/pingcap/pd/server/schedule"
	"time"
)

type leaderFilter struct {
}

func NewLeaderFilter() schedule.RegionFilter {
	return &leaderFilter{}
}

func (f *leaderFilter) Type() string {
	return "leader-filter"
}

func (f *leaderFilter) FilterSource(opt schedule.Options, region *core.RegionInfo, interval *schedule.TimeInterval, regionIDs []uint64) bool {
	if interval != nil {
		currentTime := time.Now()
		if currentTime.After(interval.End) || interval.Begin.After(currentTime) {
			return false
		}
	}
	return f.isExist(region.GetID(), regionIDs)
}

func (f *leaderFilter) FilterTarget(opt schedule.Options, region *core.RegionInfo, interval *schedule.TimeInterval, regionIDs []uint64) bool {
	if interval != nil {
		currentTime := time.Now()
		if currentTime.After(interval.End) || interval.Begin.After(currentTime) {
			return false
		}
	}
	return f.isExist(region.GetID(), regionIDs)
}

func (f *leaderFilter) isExist(regionID uint64, regionIDs []uint64) bool {
	for _, id := range regionIDs {
		if id == regionID {
			return true
		}
	}
	return false
}
