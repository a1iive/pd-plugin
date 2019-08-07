// Copyright 2017 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package schedulers

import (
	"strconv"
	"strings"

	"github.com/pingcap/log"
	"github.com/pingcap/pd/pkg/cache"
	"github.com/pingcap/pd/server/core"
	"github.com/pingcap/pd/server/schedule"
	"go.uber.org/zap"
)

func init() {
	schedule.RegisterScheduler("balance-leader", func(opController *schedule.OperatorController, args []string) (schedule.Scheduler, error) {
		return newBalanceLeaderScheduler(opController), nil
	})
}

// balanceLeaderRetryLimit is the limit to retry schedule for selected source store and target store.
const balanceLeaderRetryLimit = 10

type balanceLeaderScheduler struct {
	*baseScheduler
	selector      *schedule.BalanceSelector
	regionFilters []schedule.RegionFilter
	taintStores   *cache.TTLUint64
	opController  *schedule.OperatorController
}

// newBalanceLeaderScheduler creates a scheduler that tends to keep leaders on
// each store balanced.
func newBalanceLeaderScheduler(opController *schedule.OperatorController) schedule.Scheduler {
	taintStores := newTaintCache()
	filters := []schedule.Filter{
		schedule.StoreStateFilter{TransferLeader: true},
		schedule.NewCacheFilter(taintStores),
	}
	regionFilters := []schedule.RegionFilter{}
	//get func from plugin
	//func : NewLeaderFilter()
	f, err := schedule.GetFunction("./plugin/testPlugin.so", "NewLeaderFilter")
	if err != nil {
		log.Error("Plugin GetFunction err", zap.Error(err))
	} else {
		NewLeaderFilter := f.(func() schedule.RegionFilter)
		regionFilters = append(regionFilters, NewLeaderFilter())
	}
	base := newBaseScheduler(opController)
	s := &balanceLeaderScheduler{
		baseScheduler: base,
		selector:      schedule.NewBalanceSelector(core.LeaderKind, filters),
		regionFilters: regionFilters,
		taintStores:   taintStores,
		opController:  opController,
	}
	return s
}

func (l *balanceLeaderScheduler) GetName() string {
	return "balance-leader-scheduler"
}

func (l *balanceLeaderScheduler) GetType() string {
	return "balance-leader"
}

func (l *balanceLeaderScheduler) IsScheduleAllowed(cluster schedule.Cluster) bool {
	return l.opController.OperatorCount(schedule.OpLeader) < cluster.GetLeaderScheduleLimit()
}

func (l *balanceLeaderScheduler) Schedule(cluster schedule.Cluster) []*schedule.Operator {
	schedulerCounter.WithLabelValues(l.GetName(), "schedule").Inc()

	stores := cluster.GetStores()

	// source/target is the store with highest/lowest leader score in the list that
	// can be selected as balance source/target.
	source := l.selector.SelectSource(cluster, stores)
	target := l.selector.SelectTarget(cluster, stores)

	// No store can be selected as source or target.
	if source == nil || target == nil {
		schedulerCounter.WithLabelValues(l.GetName(), "no_store").Inc()
		// When the cluster is balanced, all stores will be added to the cache once
		// all of them have been selected. This will cause the scheduler to not adapt
		// to sudden change of a store's leader. Here we clear the taint cache and
		// re-iterate.
		l.taintStores.Clear()
		return nil
	}

	sourceID := source.GetID()
	targetID := target.GetID()
	log.Debug("store leader score", zap.String("scheduler", l.GetName()), zap.Uint64("max-store", sourceID), zap.Uint64("min-store", targetID))
	sourceStoreLabel := strconv.FormatUint(sourceID, 10)
	targetStoreLabel := strconv.FormatUint(targetID, 10)
	sourceAddress := source.GetAddress()
	targetAddress := target.GetAddress()
	balanceLeaderCounter.WithLabelValues("high_score", sourceAddress, sourceStoreLabel).Inc()
	balanceLeaderCounter.WithLabelValues("low_score", targetAddress, targetStoreLabel).Inc()

	opInfluence := l.opController.GetOpInfluence(cluster)
	for i := 0; i < balanceLeaderRetryLimit; i++ {
		if op := l.transferLeaderOut(source, cluster, opInfluence); op != nil {
			balanceLeaderCounter.WithLabelValues("transfer_out", sourceAddress, sourceStoreLabel).Inc()
			return op
		}
		if op := l.transferLeaderIn(target, cluster, opInfluence); op != nil {
			balanceLeaderCounter.WithLabelValues("transfer_in", targetAddress, targetStoreLabel).Inc()
			return op
		}
	}

	// If no operator can be created for the selected stores, ignore them for a while.
	log.Debug("no operator created for selected stores", zap.String("scheduler", l.GetName()), zap.Uint64("source", sourceID), zap.Uint64("target", targetID))
	balanceLeaderCounter.WithLabelValues("add_taint", sourceAddress, sourceStoreLabel).Inc()
	l.taintStores.Put(sourceID)
	balanceLeaderCounter.WithLabelValues("add_taint", targetAddress, targetStoreLabel).Inc()
	l.taintStores.Put(targetID)
	return nil
}

// transferLeaderOut transfers leader from the source store.
// It randomly selects a health region from the source store, then picks
// the best follower peer and transfers the leader.
func (l *balanceLeaderScheduler) transferLeaderOut(source *core.StoreInfo, cluster schedule.Cluster, opInfluence schedule.OpInfluence) []*schedule.Operator {
	schedule.PluginsMapLock.RLock()
	defer schedule.PluginsMapLock.RUnlock()
	sourceID := source.GetID()
	region := cluster.RandLeaderRegion(sourceID, core.HealthRegion())

	if region == nil {
		log.Debug("store has no leader", zap.String("scheduler", l.GetName()), zap.Uint64("store-id", sourceID))
		schedulerCounter.WithLabelValues(l.GetName(), "no_leader_region").Inc()
		return nil
	}

	//过滤与用户leader调度有冲突的region
	if len(l.regionFilters) != 0 {
		for str, pluginInfo := range schedule.PluginsMap {
			s := strings.Split(str, "-")
			if s[0] == "Leader" && schedule.RegionFilterSource(cluster, region, l.regionFilters, pluginInfo.GetInterval(), pluginInfo.GetRegionIDs()) {
				return nil
			}
		}
	}

	target := l.selector.SelectTarget(cluster, cluster.GetFollowerStores(region))
	if target == nil {
		log.Debug("region has no target store", zap.String("scheduler", l.GetName()), zap.Uint64("region-id", region.GetID()))
		schedulerCounter.WithLabelValues(l.GetName(), "no_target_store").Inc()
		return nil
	}
	return l.createOperator(region, source, target, cluster, opInfluence)
}

// transferLeaderIn transfers leader to the target store.
// It randomly selects a health region from the target store, then picks
// the worst follower peer and transfers the leader.
func (l *balanceLeaderScheduler) transferLeaderIn(target *core.StoreInfo, cluster schedule.Cluster, opInfluence schedule.OpInfluence) []*schedule.Operator {
	schedule.PluginsMapLock.RLock()
	defer schedule.PluginsMapLock.RUnlock()

	targetID := target.GetID()
	region := cluster.RandFollowerRegion(targetID, core.HealthRegion())

	if region == nil {
		log.Debug("store has no follower", zap.String("scheduler", l.GetName()), zap.Uint64("store-id", targetID))
		schedulerCounter.WithLabelValues(l.GetName(), "no_follower_region").Inc()
		return nil
	}

	//过滤与用户leader调度有冲突的region
	if len(l.regionFilters) != 0 {
		for str, pluginInfo := range schedule.PluginsMap {
			s := strings.Split(str, "-")
			if s[0] == "Leader" && schedule.RegionFilterSource(cluster, region, l.regionFilters, pluginInfo.GetInterval(), pluginInfo.GetRegionIDs()) {
				return nil
			}
		}
	}

	source := cluster.GetStore(region.GetLeader().GetStoreId())
	if source == nil {
		log.Debug("region has no leader", zap.String("scheduler", l.GetName()), zap.Uint64("region-id", region.GetID()))
		schedulerCounter.WithLabelValues(l.GetName(), "no_leader").Inc()
		return nil
	}
	return l.createOperator(region, source, target, cluster, opInfluence)
}

// createOperator creates the operator according to the source and target store.
// If the region is hot or the difference between the two stores is tolerable, then
// no new operator need to be created, otherwise create an operator that transfers
// the leader from the source store to the target store for the region.
func (l *balanceLeaderScheduler) createOperator(region *core.RegionInfo, source, target *core.StoreInfo, cluster schedule.Cluster, opInfluence schedule.OpInfluence) []*schedule.Operator {
	regionID := region.GetID()
	if cluster.IsRegionHot(regionID) {
		log.Debug("region is hot region, ignore it", zap.String("scheduler", l.GetName()), zap.Uint64("region-id", regionID))
		schedulerCounter.WithLabelValues(l.GetName(), "region_hot").Inc()
		return nil
	}

	sourceID := source.GetID()
	targetID := target.GetID()
	if !shouldBalance(cluster, source, target, region, core.LeaderKind, opInfluence) {
		log.Debug("skip balance leader",
			zap.String("scheduler", l.GetName()), zap.Uint64("region-id", regionID), zap.Uint64("source-store", sourceID), zap.Uint64("target-store", targetID),
			zap.Int64("source-size", source.GetLeaderSize()), zap.Float64("source-score", source.LeaderScore(0)),
			zap.Int64("source-influence", opInfluence.GetStoreInfluence(sourceID).ResourceSize(core.LeaderKind)),
			zap.Int64("target-size", target.GetLeaderSize()), zap.Float64("target-score", target.LeaderScore(0)),
			zap.Int64("target-influence", opInfluence.GetStoreInfluence(targetID).ResourceSize(core.LeaderKind)),
			zap.Int64("average-region-size", cluster.GetAverageRegionSize()))
		schedulerCounter.WithLabelValues(l.GetName(), "skip").Inc()
		return nil
	}

	schedulerCounter.WithLabelValues(l.GetName(), "new_operator").Inc()
	sourceLabel := strconv.FormatUint(sourceID, 10)
	targetLabel := strconv.FormatUint(targetID, 10)
	balanceLeaderCounter.WithLabelValues("move_leader", source.GetAddress()+"-out", sourceLabel).Inc()
	balanceLeaderCounter.WithLabelValues("move_leader", target.GetAddress()+"-in", targetLabel).Inc()
	op := schedule.CreateTransferLeaderOperator("balance-leader", region, region.GetLeader().GetStoreId(), targetID, schedule.OpBalance)
	return []*schedule.Operator{op}
}
