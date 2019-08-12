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

package checker

import (
	"github.com/pingcap/kvproto/pkg/metapb"
	"github.com/pingcap/log"
	"github.com/pingcap/pd/server/core"
	"github.com/pingcap/pd/server/namespace"
	"github.com/pingcap/pd/server/schedule"
	"go.uber.org/zap"
)

// NamespaceChecker ensures region to go to the right place.
type NamespaceChecker struct {
	cluster       schedule.Cluster
	filters       []schedule.Filter
	regionFilters []schedule.RegionFilter
	classifier    namespace.Classifier
}

// NewNamespaceChecker creates a namespace checker.
func NewNamespaceChecker(cluster schedule.Cluster, classifier namespace.Classifier) *NamespaceChecker {
	filters := []schedule.Filter{
		schedule.StoreStateFilter{MoveRegion: true},
	}
	regionFilters := []schedule.RegionFilter{}
	//get func from plugin
	//func : NewViolentFilter()
	f, err := schedule.GetFunction("./plugin/userConfigPlugin.so", "NewViolentFilter")
	if err != nil {
		log.Error("Plugin GetFunction err", zap.Error(err))
	} else {
		NewViolentFilter := f.(func() schedule.RegionFilter)
		regionFilters = append(regionFilters, NewViolentFilter())
	}

	return &NamespaceChecker{
		cluster:       cluster,
		filters:       filters,
		regionFilters: regionFilters,
		classifier:    classifier,
	}
}

// Check verifies a region's namespace, creating an Operator if need.
func (n *NamespaceChecker) Check(region *core.RegionInfo) *schedule.Operator {
	schedule.PluginsMapLock.RLock()
	defer schedule.PluginsMapLock.RUnlock()

	if !n.cluster.IsNamespaceRelocationEnabled() {
		return nil
	}
	//skip user regions
	if len(n.regionFilters) != 0 {
		for _, pluginInfo := range schedule.PluginsMap {
			regionIDs := schedule.GetRegionIDs(n.cluster, pluginInfo.GetKeyStart(), pluginInfo.GetKeyEnd())
			if schedule.RegionFilterSource(n.cluster, region, n.regionFilters, pluginInfo.GetInterval(), regionIDs) {
				return nil
			}
		}
	}

	checkerCounter.WithLabelValues("namespace_checker", "check").Inc()

	// fail-fast if there is only ONE namespace
	if n.classifier == nil || len(n.classifier.GetAllNamespaces()) == 1 {
		checkerCounter.WithLabelValues("namespace_checker", "no_namespace").Inc()
		return nil
	}

	// get all the stores belong to the namespace
	targetStores := n.getNamespaceStores(region)
	if len(targetStores) == 0 {
		checkerCounter.WithLabelValues("namespace_checker", "no_target_store").Inc()
		return nil
	}
	for _, peer := range region.GetPeers() {
		// check whether the peer has been already located on a store that is belong to the target namespace
		if n.isExists(targetStores, peer.StoreId) {
			continue
		}
		log.Debug("peer is not located in namespace target stores", zap.Uint64("region-id", region.GetID()), zap.Reflect("peer", peer))
		newPeer := n.SelectBestPeerToRelocate(region, targetStores)
		if newPeer == nil {
			checkerCounter.WithLabelValues("namespace_checker", "no_target_peer").Inc()
			return nil
		}
		op, err := schedule.CreateMovePeerOperator("make-namespace-relocation", n.cluster, region, schedule.OpReplica, peer.GetStoreId(), newPeer.GetStoreId(), newPeer.GetId())
		if err != nil {
			checkerCounter.WithLabelValues("namespace_checker", "create_operator_fail").Inc()
			return nil
		}
		checkerCounter.WithLabelValues("namespace_checker", "new_operator").Inc()
		return op
	}

	checkerCounter.WithLabelValues("namespace_checker", "all_right").Inc()
	return nil
}

// SelectBestPeerToRelocate return a new peer that to be used to move a region
func (n *NamespaceChecker) SelectBestPeerToRelocate(region *core.RegionInfo, targets []*core.StoreInfo) *metapb.Peer {
	storeID := n.SelectBestStoreToRelocate(region, targets)
	if storeID == 0 {
		log.Debug("has no best store to relocate", zap.Uint64("region-id", region.GetID()))
		return nil
	}
	newPeer, err := n.cluster.AllocPeer(storeID)
	if err != nil {
		return nil
	}
	return newPeer
}

// SelectBestStoreToRelocate randomly returns the store to relocate
func (n *NamespaceChecker) SelectBestStoreToRelocate(region *core.RegionInfo, targets []*core.StoreInfo) uint64 {
	selector := schedule.NewRandomSelector(n.filters)
	target := selector.SelectTarget(n.cluster, targets, schedule.NewExcludedFilter(nil, region.GetStoreIds()))
	if target == nil {
		return 0
	}
	return target.GetID()
}

func (n *NamespaceChecker) isExists(stores []*core.StoreInfo, storeID uint64) bool {
	for _, store := range stores {
		if store.GetID() == storeID {
			return true
		}
	}
	return false
}

func (n *NamespaceChecker) getNamespaceStores(region *core.RegionInfo) []*core.StoreInfo {
	ns := n.classifier.GetRegionNamespace(region)
	filteredStores := n.filter(n.cluster.GetStores(), schedule.NewNamespaceFilter(n.classifier, ns))

	return filteredStores
}

func (n *NamespaceChecker) filter(stores []*core.StoreInfo, filters ...schedule.Filter) []*core.StoreInfo {
	result := make([]*core.StoreInfo, 0)

	for _, store := range stores {
		if schedule.FilterTarget(n.cluster, store, filters) {
			continue
		}
		result = append(result, store)
	}
	return result
}
