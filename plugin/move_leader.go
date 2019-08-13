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
	keyStart     string
	keyEnd       string
	storeSeq     int
	timeInterval *schedule.TimeInterval
}

// Only use for register scheduler
// newMoveLeaderUserScheduler() will be called manually
func init() {
	schedule.RegisterScheduler("move-leader-user", func(opController *schedule.OperatorController, args []string) (schedule.Scheduler, error) {
		return newMoveLeaderUserScheduler(opController, "", "", "", []uint64{}, nil), nil
	})
}

func newMoveLeaderUserScheduler(opController *schedule.OperatorController, name, keyStart, keyEnd string, storeIDs []uint64, interval *schedule.TimeInterval) schedule.Scheduler {
	log.Info("", zap.String("New", name), zap.Strings("key range", []string{keyStart, keyEnd}))
	base := newUserBaseScheduler(opController)
	return &moveLeaderUserScheduler{
		userBaseScheduler: base,
		name:              name,
		regionIDs:         []uint64{},
		storeIDs:          storeIDs,
		keyStart:          keyStart,
		keyEnd:            keyEnd,
		storeSeq:          0,
		timeInterval:      interval,
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
	// Determine if there is a time limit
	if l.timeInterval != nil {
		currentTime := time.Now()
		if currentTime.After(l.timeInterval.GetEnd()) || l.timeInterval.GetBegin().After(currentTime) {
			return nil
		}
	}
	// When region ids change, re-output scheduler's regions and stores
	output := false
	newRegionIDs := schedule.GetRegionIDs(cluster, l.keyStart, l.keyEnd)
	if len(l.regionIDs) != len(newRegionIDs){
		output = true
	}else{
		for i, oldRegionID := range l.regionIDs{
			if newRegionIDs[i] != oldRegionID{
				output = true 
				break
			}
		}
	}
	l.regionIDs = newRegionIDs
	if output{
		log.Info("", zap.String("schedule()", l.GetName()), zap.Uint64s("Regions", l.regionIDs))
		log.Info("", zap.String("schedule()", l.GetName()), zap.Uint64s("Stores", l.storeIDs))
	}
	
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
		// If leader is in target stores,
		// it means user's rules has been met,
		// then do nothing
		if !l.isExists(sourceID, l.storeIDs) {
			// Let "seq" store be the target first
			targetID := l.storeIDs[l.storeSeq]
			// If move-region and move-leader have conflict, 
			// it means the conflict is adjustable,
			// then find the overlapped stores and choose the first one as final target store
			for str, pluginInfo := range schedule.PluginsMap {
				s := strings.Split(str, "-")
				if s[0] == "Region" {
					regionIDs := schedule.GetRegionIDs(cluster, pluginInfo.GetKeyStart(), pluginInfo.GetKeyEnd())
					if l.isExists(regionID, regionIDs) {
						if ((l.timeInterval.GetBegin().Before(pluginInfo.GetInterval().GetBegin()) ||
								l.timeInterval.GetBegin().Equal(pluginInfo.GetInterval().GetBegin())) &&
								l.timeInterval.GetEnd().After(pluginInfo.GetInterval().GetBegin())) ||
							((pluginInfo.GetInterval().GetBegin().Before(l.timeInterval.GetBegin()) || 
								pluginInfo.GetInterval().GetBegin().Equal(l.timeInterval.GetBegin())) && 
								pluginInfo.GetInterval().GetEnd().After(l.timeInterval.GetBegin())) {
							overlap := IfOverlap(l.storeIDs, pluginInfo.GetStoreIDs())
							if len(overlap) != 0 {
								targetID = overlap[0]
								break
							}
						}
					}
				}
			}
			// seq increase
			if l.storeSeq < len(l.storeIDs)-1 {
				l.storeSeq++
			} else {
				l.storeSeq = 0
			}
			target := cluster.GetStore(targetID)
			if _, ok := region.GetStoreIds()[targetID]; ok {
				// target store has region peer, so do "transfer leader"
				filters := []schedule.Filter{
					schedule.StoreStateFilter{TransferLeader: true},
				}
				if schedule.FilterSource(cluster, source, filters) {
					log.Info("filter source", 
						zap.String("scheduler", l.GetName()),
						zap.Uint64("region-id", regionID),
						zap.Uint64("store-id", sourceID))
					continue
				}
				if schedule.FilterTarget(cluster, target, filters) {
					log.Info("filter target", 
						zap.String("scheduler", l.GetName()),
						zap.Uint64("region-id", regionID),
						zap.Uint64("store-id", targetID))
					continue
				}
				op := schedule.CreateTransferLeaderOperator("move-leader-user", region, sourceID, targetID, schedule.OpLeader)
				op.SetPriorityLevel(core.HighPriority)
				return []*schedule.Operator{op}
			} else {
				// target store doesn't have region peer, so do "move leader"
				filters := []schedule.Filter{
					schedule.StoreStateFilter{MoveRegion: true},
				}
				if schedule.FilterSource(cluster, source, filters) {
					log.Info("filter source", 
						zap.String("scheduler", l.GetName()),
						zap.Uint64("region-id", regionID),
						zap.Uint64("store-id", sourceID))
					continue
				}
				if schedule.FilterTarget(cluster, target, filters) {
					log.Info("filter target", 
						zap.String("scheduler", l.GetName()),
						zap.Uint64("region-id", regionID),
						zap.Uint64("store-id", targetID))
					continue
				}
				destPeer, err := cluster.AllocPeer(targetID)
				if err != nil {
					log.Error("failed to allocate peer", zap.Error(err))
					continue
				}
				op, err := schedule.CreateMoveLeaderOperator("move-leader-user", cluster, region, schedule.OpAdmin, sourceID, targetID, destPeer.GetId())
				if err != nil {
					log.Error("CreateMoveLeaderOperator Err", 
						zap.String("scheduler", l.GetName()),
						zap.Error(err))
					continue
				}
				op.SetPriorityLevel(core.HighPriority)
				return []*schedule.Operator{op}
			}
		}
	}
	return nil
}

// isExists(ID , IDs) determine if the ID is in IDs
func (l *moveLeaderUserScheduler) isExists(ID uint64, IDs []uint64) bool {
	for _, id := range IDs {
		if id == ID {
			return true
		}
	}
	return false
}
