package main

import (
<<<<<<< HEAD
	"github.com/pingcap/pd/server/core"
	"github.com/pingcap/pd/server/schedule"
	"github.com/pingcap/log"
	"go.uber.org/zap"
=======
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
		return newMoveRegionUserScheduler(opController, []uint64{}, []uint64{}, nil), nil

=======
		return newMoveRegionUserScheduler(opController, "", []uint64{}, []uint64{}, nil), nil
>>>>>>> dev
	})
}

func newMoveRegionUserScheduler(opController *schedule.OperatorController, name string, regionIDs []uint64, storeIDs []uint64, interval *schedule.TimeInterval) schedule.Scheduler {
	filters := []schedule.Filter{
		schedule.StoreStateFilter{MoveRegion: true},
	}
	base := newUserBaseScheduler(opController)
	return &moveRegionUserScheduler{
		userBaseScheduler: base,
		name:              name,
		regionIDs:         regionIDs,
		storeIDs:          storeIDs,
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
	if r.timeInterval != nil {
		currentTime := time.Now()
		if currentTime.After(r.timeInterval.End) || r.timeInterval.Begin.After(currentTime) {
			return nil
		}
	}
	
	log.Info("move region schedule run", zap.Int("len of region", len(r.regionIDs)))
	log.Info("move region schedule run", zap.Int("len of store", len(r.storeIDs)))
	var ops []*schedule.Operator

	if len(r.storeIDs) == 0 {
		return ops
	}

	for _, regionID := range r.regionIDs {
<<<<<<< HEAD
		log.Info("move region schedule", zap.Uint64("try region", regionID))
		region := cluster.GetRegion(regionID)
		if !r.allExist(r.storeIDs, region){
			replicas := len(region.GetStoreIds())
			for storeID := range region.GetStoreIds() {
				if replicas > len(r.storeIDs) {
					if _, ok := storeIDs[storeID]; !ok {
						storeIDs[storeID] = struct{}{}
					}
				} else {
=======
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
				}else {
>>>>>>> dev
					break
				}
			}
			op, err := schedule.CreateMoveRegionOperator("move-region-user", cluster, region, schedule.OpAdmin, storeIDs)
			if err != nil {
				continue
			}
			ops = append(ops, op)
			return ops
		}
	}
	return ops
}

<<<<<<< HEAD
func (r *moveRegionUserScheduler) allExist(storeIDs []uint64,region *core.RegionInfo) bool{
	for _, storeID := range storeIDs{
		if _, ok := region.GetStoreIds()[storeID]; ok{
			continue
		}else {
=======
func (r *moveRegionUserScheduler) allExist(storeIDs []uint64, region *core.RegionInfo) bool {
	for _, storeID := range storeIDs {
		if _, ok := region.GetStoreIds()[storeID]; ok {
			continue
		} else {
>>>>>>> dev
			return false
		}
	}
	return true
}
