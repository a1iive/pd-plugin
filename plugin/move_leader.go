package main

import (
	"strings"
	"time"

	"github.com/pingcap/log"
	"github.com/pingcap/pd/server/core"
	"github.com/pingcap/pd/server/schedule"
	"go.uber.org/zap"
)

type moveLeaderUserScheduler struct {
	*userBaseScheduler
	name         string
	opController *schedule.OperatorController
	regionIDs    []uint64
	storeIDs     []uint64
	storeSeq     int
	timeInterval *schedule.TimeInterval
	regionFilter []schedule.RegionFilter
	filters      []schedule.Filter
}

func init() {
	schedule.RegisterScheduler("move-leader-user", func(opController *schedule.OperatorController, args []string) (schedule.Scheduler, error) {
		return newMoveLeaderUserScheduler(opController, "", nil), nil
	})
}

func newMoveLeaderUserScheduler(opController *schedule.OperatorController, name string, interval *schedule.TimeInterval) schedule.Scheduler {
	filters := []schedule.Filter{
		schedule.StoreStateFilter{TransferLeader: true},
	}
	regionFilters := []schedule.RegionFilter{NewLeaderFilter()}
	base := newUserBaseScheduler(opController)
	return &moveLeaderUserScheduler{
		userBaseScheduler: base,
		name:              name,
		regionIDs:         []uint64{},
		storeIDs:          []uint64{},
		storeSeq:          0,
		timeInterval:      interval,
		filters:           filters,
		regionFilter:      regionFilters,
		opController:      opController,
	}
}

func (l *moveLeaderUserScheduler) GetName() string {
	return l.name
}

func (l *moveLeaderUserScheduler) GetType() string {
	return "move-leader-user"
}

func (l *moveLeaderUserScheduler) IsScheduleAllowed(cluster schedule.Cluster) bool {
	return l.opController.OperatorCount(schedule.OpLeader) < cluster.GetLeaderScheduleLimit()
}

func (l *moveLeaderUserScheduler) Schedule(cluster schedule.Cluster) []*schedule.Operator {
	schedule.PluginsMapLock.RLock()
	defer schedule.PluginsMapLock.RUnlock()

	if l.timeInterval != nil {
		currentTime := time.Now()
		if currentTime.After(l.timeInterval.End) || l.timeInterval.Begin.After(currentTime) {
			return nil
		}
	}

	ss := strings.Split(l.name, "-")
	l.regionIDs = schedule.PluginsMap["Leader-"+ss[4]].GetRegionIDs()
	l.storeIDs = schedule.PluginsMap["Leader-"+ss[4]].GetStoreIDs()

	if len(l.storeIDs) == 0 {
		return nil
	}
	for _, regionID := range l.regionIDs {
		region := cluster.GetRegion(regionID)
		if region == nil {
			log.Info("region not exist", zap.Uint64("region-id", regionID))
			continue
		}
		sourceID := region.GetLeader().GetStoreId()
		source := cluster.GetStore(sourceID)
		if schedule.FilterSource(cluster, source, l.filters) {
			continue
		}
		//如果leader不在选定stores上
		if !l.isExists(sourceID, l.storeIDs) {
			targetID := l.storeIDs[l.storeSeq]
			for str, pluginInfo := range schedule.PluginsMap {
				s := strings.Split(str, "-")
				if s[0] == "Region" {
					if l.isExists(regionID, pluginInfo.GetRegionIDs()) {
						overlap := IfOverlap(l.storeIDs, pluginInfo.GetStoreIDs())
						if len(overlap) != 0 {
							targetID = overlap[0]
							break
						}
					}
				}
			}
			if l.storeSeq < len(l.storeIDs)-1 {
				l.storeSeq++
			} else {
				l.storeSeq = 0
			}
			target := cluster.GetStore(targetID)
			if schedule.FilterTarget(cluster, target, l.filters) {
				continue
			}
			if _, ok := region.GetStoreIds()[targetID]; ok {
				//target store has region peer, so transfer leader
				op := schedule.CreateTransferLeaderOperator("move-leader-user", region, sourceID, targetID, schedule.OpLeader)
				op.SetPriorityLevel(core.HighPriority)
				return []*schedule.Operator{op}
			} else {
				//target store doesn't have region peer, so move leader
				destPeer, err := cluster.AllocPeer(targetID)
				if err != nil {
					log.Error("failed to allocate peer", zap.Error(err))
					continue
				}
				op, err := schedule.CreateMoveLeaderOperator("move-leader-user", cluster, region, schedule.OpAdmin, sourceID, targetID, destPeer.GetId())
				if err != nil {
					continue
				}
				op.SetPriorityLevel(core.HighPriority)
				return []*schedule.Operator{op}
			}
		}
	}
	return nil
}

func (l *moveLeaderUserScheduler) isExists(ID uint64, IDs []uint64) bool {
	for _, id := range IDs {
		if id == ID {
			return true
		}
	}
	return false
}
