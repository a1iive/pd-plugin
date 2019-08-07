package main

import (
	"github.com/pingcap/pd/server/schedule"
	"time"
)

type moveRegionUserScheduler struct {
	*userBaseScheduler
	regionIDs    []uint64
	storeIDs     []uint64
	timeInterval *schedule.TimeInterval
	filters      []schedule.Filter
}

func init() {
	schedule.RegisterScheduler("move-region-user", func(opController *schedule.OperatorController, args []string) (schedule.Scheduler, error) {
		return newMoveRegionUserScheduler(opController, []uint64{}, []uint64{}, nil), nil
	})
}

func newMoveRegionUserScheduler(opController *schedule.OperatorController, regionIDs []uint64, storeIDs []uint64, interval *schedule.TimeInterval) schedule.Scheduler {
	filters := []schedule.Filter{
		schedule.StoreStateFilter{MoveRegion: true},
	}
	base := newUserBaseScheduler(opController)
	userScheduler2 = moveRegionUserScheduler{
		userBaseScheduler: base,
		regionIDs:         regionIDs,
		storeIDs:          storeIDs,
		timeInterval:      interval,
		filters:           filters,
	}
	return &userScheduler2
}

func (r *moveRegionUserScheduler) GetName() string {
	return "move-region-user-scheduler"
}

func (r *moveRegionUserScheduler) GetType() string {
	return "move-region-user"
}

func (r *moveRegionUserScheduler) IsScheduleAllowed(cluster schedule.Cluster) bool {
	return r.opController.OperatorCount(schedule.OpRegion) < cluster.GetRegionScheduleLimit()
}

func (r *moveRegionUserScheduler) Schedule(cluster schedule.Cluster) []*schedule.Operator {
	if r.timeInterval != nil {
		currentTime := time.Now()
		if currentTime.After(r.timeInterval.End) || r.timeInterval.Begin.After(currentTime) {
			return nil
		}
	}
	var ops []*schedule.Operator
	var storeIDs = make(map[uint64]struct{})

	if len(r.storeIDs) == 0 {
		return ops
	}

	for _, storeID := range r.storeIDs {
		storeIDs[storeID] = struct{}{}
	}

	for _, regionID := range r.regionIDs {
		region := cluster.GetRegion(regionID)
		replicas := len(region.GetStoreIds())
		for storeID := range region.GetStoreIds() {
			if replicas > len(r.storeIDs) {
				if _, ok := storeIDs[storeID]; !ok {
					storeIDs[storeID] = struct{}{}
				}
			} else {
				break
			}
		}

		op, err := schedule.CreateMoveRegionOperator("move-region-user", cluster, region, schedule.OpAdmin, storeIDs)
		if err != nil {
			continue
		}
		ops = append(ops, op)
	}
	return ops
}

var userScheduler2 moveRegionUserScheduler
