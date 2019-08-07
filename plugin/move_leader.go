package main

import (
	"time"

	"github.com/pingcap/log"
	"github.com/pingcap/pd/server/core"
	"github.com/pingcap/pd/server/schedule"
	"go.uber.org/zap"
)

var storeSeq = 0

type moveLeaderUserScheduler struct {
	*userBaseScheduler
	opController *schedule.OperatorController
	regionIDs    []uint64
	storeIDs     []uint64
	timeInterval *schedule.TimeInterval
	filters      []schedule.Filter
}

func init() {
	schedule.RegisterScheduler("move-leader-user", func(opController *schedule.OperatorController, args []string) (schedule.Scheduler, error) {
		return newMoveLeaderUserScheduler(opController, []uint64{}, []uint64{}, nil), nil
	})
}

func newMoveLeaderUserScheduler(opController *schedule.OperatorController, regionIDs []uint64, storeIDs []uint64, interval *schedule.TimeInterval) schedule.Scheduler {
	filters := []schedule.Filter{
		schedule.StoreStateFilter{TransferLeader: true},
	}
	base := newUserBaseScheduler(opController)

	return &moveLeaderUserScheduler{
		userBaseScheduler: base,
		opController:	   opController,
		regionIDs:         regionIDs,
		storeIDs:          storeIDs,
		timeInterval:      interval,
		filters:           filters,
	}
}

func (l *moveLeaderUserScheduler) GetName() string {
	return "move-leader-user-scheduler"
}

func (l *moveLeaderUserScheduler) GetType() string {
	return "move-leader-user"
}

func (l *moveLeaderUserScheduler) IsScheduleAllowed(cluster schedule.Cluster) bool {
	return l.opController.OperatorCount(schedule.OpLeader) < cluster.GetLeaderScheduleLimit()
}

func (l *moveLeaderUserScheduler) Schedule(cluster schedule.Cluster) []*schedule.Operator {
	if l.timeInterval != nil {
		currentTime := time.Now()
		if currentTime.After(l.timeInterval.End) || l.timeInterval.Begin.After(currentTime) {
			log.Info("time not suitable")
			return nil
		}
	}

	var ops []*schedule.Operator
	if len(l.storeIDs) == 0 {
		return ops
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
			log.Info("source store has been filtered", zap.Uint64("store-id", sourceID))
			continue
		}
		//如果leader不在选定stores上
		if !l.isExist(sourceID, l.storeIDs) {
			targetID := l.storeIDs[storeSeq]
			if storeSeq < len(l.storeIDs)-1 {
                                storeSeq++
                        } else {
                                storeSeq = 0
                        }
			target := cluster.GetStore(targetID)
			if schedule.FilterTarget(cluster, target, l.filters) {
				log.Info("target store has been filtered", zap.Uint64("store-id", targetID))
				continue
			}
			if _, ok := region.GetStoreIds()[targetID]; ok {
				//target store has region peer, so transfer leader
				op := schedule.CreateTransferLeaderOperator("move-leader-user", region, sourceID, targetID, schedule.OpLeader)
				op.SetPriorityLevel(core.HighPriority)
				ops = append(ops, op)
				return ops
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
				ops = append(ops, op)
				return ops
			}
		}

	}
	return ops
}

func (l *moveLeaderUserScheduler) isExist(storeID uint64, storeIDs []uint64) bool {
	for _, id := range storeIDs {
		if id == storeID {
			return true
		}
	}
	return false
}

var userScheduler1 moveLeaderUserScheduler
