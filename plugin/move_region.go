package main

import (
<<<<<<< HEAD
<<<<<<< HEAD
	"github.com/pingcap/pd/server/core"
	"github.com/pingcap/pd/server/schedule"
	"github.com/pingcap/log"
	"go.uber.org/zap"
=======
>>>>>>> dev
=======
	"strings"
>>>>>>> dev
	"time"

	"github.com/pingcap/log"
	"github.com/pingcap/pd/server/core"
	"github.com/pingcap/pd/server/schedule"
	"go.uber.org/zap"
)

type moveRegionUserScheduler struct {
	*userBaseScheduler
<<<<<<< HEAD
=======
	name         string
>>>>>>> dev
	opController *schedule.OperatorController
	regionIDs    []uint64
	storeIDs     []uint64
	timeInterval *schedule.TimeInterval
	filters      []schedule.Filter
}

func init() {
	schedule.RegisterScheduler("move-region-user", func(opController *schedule.OperatorController, args []string) (schedule.Scheduler, error) {
<<<<<<< HEAD
<<<<<<< HEAD
		return newMoveRegionUserScheduler(opController, []uint64{}, []uint64{}, nil), nil

=======
		return newMoveRegionUserScheduler(opController, "", []uint64{}, []uint64{}, nil), nil
>>>>>>> dev
=======
		return newMoveRegionUserScheduler(opController, "", nil), nil
>>>>>>> dev
	})
}

func newMoveRegionUserScheduler(opController *schedule.OperatorController, name string, interval *schedule.TimeInterval) schedule.Scheduler {
	filters := []schedule.Filter{
		schedule.StoreStateFilter{MoveRegion: true},
	}
	base := newUserBaseScheduler(opController)
	return &moveRegionUserScheduler{
		userBaseScheduler: base,
		name:              name,
		regionIDs:         []uint64{},
		storeIDs:          []uint64{},
		timeInterval:      interval,
		filters:           filters,
		opController:      opController,
	}
}

func (r *moveRegionUserScheduler) GetName() string {
	return r.name
}

func (r *moveRegionUserScheduler) GetType() string {
	return "move-region-user"
}

func (r *moveRegionUserScheduler) IsScheduleAllowed(cluster schedule.Cluster) bool {
	return r.opController.OperatorCount(schedule.OpRegion) < cluster.GetRegionScheduleLimit()
}

func (r *moveRegionUserScheduler) Schedule(cluster schedule.Cluster) []*schedule.Operator {
	schedule.PluginsMapLock.RLock()
	defer schedule.PluginsMapLock.RUnlock()

	if r.timeInterval != nil {
		currentTime := time.Now()
		if currentTime.After(r.timeInterval.End) || r.timeInterval.Begin.After(currentTime) {
			return nil
		}
	}
	ss := strings.Split(r.name, "-")
	r.regionIDs = schedule.PluginsMap["Region-"+ss[4]].GetRegionIDs()
	r.storeIDs = schedule.PluginsMap["Region-"+ss[4]].GetStoreIDs()

	if len(r.storeIDs) == 0 {
		return nil
	}

	for _, regionID := range r.regionIDs {
		storeIDs := make(map[uint64]struct{})
		for _, storeID := range r.storeIDs {
			storeIDs[storeID] = struct{}{}
		}
		region := cluster.GetRegion(regionID)
		if region == nil {
			log.Info("region not exist", zap.Uint64("region-id", regionID))
			continue
		}
		if !r.allExist(r.storeIDs, region) {
			replicas := cluster.GetMaxReplicas()
			for storeID := range region.GetStoreIds() {
				if replicas > len(storeIDs) {
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
			return []*schedule.Operator{op}
		}
	}
	return nil
}

func (r *moveRegionUserScheduler) allExist(storeIDs []uint64, region *core.RegionInfo) bool {
	for _, storeID := range storeIDs {
		if _, ok := region.GetStoreIds()[storeID]; ok {
			continue
		} else {
			return false
		}
	}
	return true
}
