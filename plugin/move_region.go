package main

import (
	"strings"
	"time"

	"github.com/pingcap/log"
	"github.com/pingcap/pd/server/core"
	"github.com/pingcap/pd/server/schedule"
	"go.uber.org/zap"
)

type moveRegionUserScheduler struct {
	*userBaseScheduler
	name         string
	opController *schedule.OperatorController
	regionIDs    []uint64
	storeIDs     []uint64
	keyStart     string
	keyEnd       string
	timeInterval *schedule.TimeInterval
}

func init() {
	schedule.RegisterScheduler("move-region-user", func(opController *schedule.OperatorController, args []string) (schedule.Scheduler, error) {
		return newMoveRegionUserScheduler(opController, "", "", "", []uint64{}, nil), nil
	})
}

func newMoveRegionUserScheduler(opController *schedule.OperatorController, name, keyStart, keyEnd string, storeIDs []uint64, interval *schedule.TimeInterval) schedule.Scheduler {
	base := newUserBaseScheduler(opController)
	log.Info("new"+name, zap.Strings("key range", []string{keyStart, keyEnd}))
	return &moveRegionUserScheduler{
		userBaseScheduler: base,
		name:              name,
		regionIDs:         []uint64{},
		storeIDs:          storeIDs,
		keyStart:          keyStart,
		keyEnd:            keyEnd,
		timeInterval:      interval,
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
		if currentTime.After(r.timeInterval.GetEnd()) || r.timeInterval.GetBegin().After(currentTime) {
			return nil
		}
	}

	r.regionIDs = schedule.GetRegionIDs(cluster, r.keyStart, r.keyEnd)
	log.Info("", zap.String("name", r.GetName()), zap.Uint64s("Regions",r.regionIDs))
	log.Info("", zap.String("name", r.GetName()), zap.Uint64s("Stores", r.storeIDs))

	if len(r.storeIDs) == 0 {
		return nil
	}
	
	filters := []schedule.Filter{
		schedule.StoreStateFilter{MoveRegion: true},
	}
	storeIDs := make(map[uint64]struct{})
	for _, storeID := range r.storeIDs {
		if schedule.FilterTarget(cluster, cluster.GetStore(storeID), filters){
			log.Info("filter target", zap.String("scheduler", r.GetName()),
				zap.Uint64("store-id", storeID))
		}else {
			storeIDs[storeID] = struct{}{}
		}
	}
	if len(storeIDs) == 0{
		return nil
	}
	for _, regionID := range r.regionIDs {
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
						if schedule.FilterTarget(cluster, cluster.GetStore(storeID), filters) {
							log.Info("filter target", zap.String("scheduler", r.GetName()),
								zap.Uint64("store-id", storeID))
						}else {
							storeIDs[storeID] = struct{}{}
						}
					}
				} else {
					break
				}
			}
			if replicas > len(storeIDs){
				log.Info("replicas > len(storeIDs)",zap.String("scheduler", r.GetName()))
				continue
			}
			op, err := schedule.CreateMoveRegionOperator("move-region-user", cluster, region, schedule.OpAdmin, storeIDs)
			if err != nil {
				log.Error("CreateMoveRegionOperator Err",zap.String("scheduler", r.GetName()),
					zap.Error(err))
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
