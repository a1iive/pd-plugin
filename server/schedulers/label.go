// Copyright 2018 PingCAP, Inc.
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
	log "github.com/pingcap/log"
	"github.com/pingcap/pd/server/core"
	"github.com/pingcap/pd/server/schedule"
	"go.uber.org/zap"
	"strings"
)

func init() {
	schedule.RegisterScheduler("label", func(opController *schedule.OperatorController, args []string) (schedule.Scheduler, error) {
		return newLabelScheduler(opController), nil
	})
}

type labelScheduler struct {
	*baseScheduler
	regionFilters []schedule.RegionFilter
	selector      *schedule.BalanceSelector
}

// LabelScheduler is mainly based on the store's label information for scheduling.
// Now only used for reject leader schedule, that will move the leader out of
// the store with the specific label.
func newLabelScheduler(opController *schedule.OperatorController) schedule.Scheduler {
	filters := []schedule.Filter{
		schedule.StoreStateFilter{TransferLeader: true},
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
	return &labelScheduler{
		baseScheduler: newBaseScheduler(opController),
		regionFilters: regionFilters,
		selector:      schedule.NewBalanceSelector(core.LeaderKind, filters),
	}
}

func (s *labelScheduler) GetName() string {
	return "label-scheduler"
}

func (s *labelScheduler) GetType() string {
	return "label"
}

func (s *labelScheduler) IsScheduleAllowed(cluster schedule.Cluster) bool {
	return s.opController.OperatorCount(schedule.OpLeader) < cluster.GetLeaderScheduleLimit()
}

func (s *labelScheduler) Schedule(cluster schedule.Cluster) []*schedule.Operator {
	schedule.PluginsMapLock.RLock()
	defer schedule.PluginsMapLock.RUnlock()
	schedulerCounter.WithLabelValues(s.GetName(), "schedule").Inc()
	stores := cluster.GetStores()
	rejectLeaderStores := make(map[uint64]struct{})
	for _, s := range stores {
		if cluster.CheckLabelProperty(schedule.RejectLeader, s.GetLabels()) {
			rejectLeaderStores[s.GetID()] = struct{}{}
		}
	}
	if len(rejectLeaderStores) == 0 {
		schedulerCounter.WithLabelValues(s.GetName(), "skip").Inc()
		return nil
	}
	log.Debug("label scheduler reject leader store list", zap.Reflect("stores", rejectLeaderStores))
	for id := range rejectLeaderStores {
		if region := cluster.RandLeaderRegion(id); region != nil {
			allow := true
			if len(s.regionFilters) != 0 {
				for str, pluginInfo := range schedule.PluginsMap{
					ss := strings.Split(str, "-")
					if ss[0] == "Leader" && schedule.RegionFilterSource(cluster, region, s.regionFilters, pluginInfo.GetInterval(), pluginInfo.GetRegionIDs()){
						allow = false
						break
					}
				}
			}
			if allow {
				log.Debug("label scheduler selects region to transfer leader", zap.Uint64("region-id", region.GetID()))
				excludeStores := make(map[uint64]struct{})
				for _, p := range region.GetDownPeers() {
					excludeStores[p.GetPeer().GetStoreId()] = struct{}{}
				}
				for _, p := range region.GetPendingPeers() {
					excludeStores[p.GetStoreId()] = struct{}{}
				}
				filter := schedule.NewExcludedFilter(nil, excludeStores)
				target := s.selector.SelectTarget(cluster, cluster.GetFollowerStores(region), filter)
				if target == nil {
					log.Debug("label scheduler no target found for region", zap.Uint64("region-id", region.GetID()))
					schedulerCounter.WithLabelValues(s.GetName(), "no_target").Inc()
					continue
				}

				schedulerCounter.WithLabelValues(s.GetName(), "new_operator").Inc()
				op := schedule.CreateTransferLeaderOperator("label-reject-leader", region, id, target.GetID(), schedule.OpLeader)
				return []*schedule.Operator{op}
			}
		}
	}
	schedulerCounter.WithLabelValues(s.GetName(), "no_region").Inc()
	return nil
}
