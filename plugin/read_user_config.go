package main

import (
	"github.com/pingcap/log"
	"github.com/pingcap/pd/server/schedule"
	"go.uber.org/zap"
	"strconv"
	"strings"
)

func ProduceScheduler(cfg schedule.Config, opController *schedule.OperatorController, cluster schedule.Cluster) []schedule.Scheduler {
	storeMap := cfg.GetStoreId(cluster)
	intervalMaps := cfg.GetInterval()
	schedules := []schedule.Scheduler{}
	pairs := cfg.IfNeedCheckStore()
	allow := true
	// if different types of rules have conflict,
	// check whether the target stores intersect,
	// and if so, the conflict is adjustable
	for _, pair := range pairs {
		l := "Leader-" + strconv.Itoa(pair[0])
		r := "Region-" + strconv.Itoa(pair[1])
		log.Info("Need Check Store", zap.Strings("Config", []string{l, r}))
		if len(IfOverlap(storeMap[l], storeMap[r])) == 0 {
			log.Error("Key Range Conflict", zap.Strings("Config", []string{l, r}))
			allow = false
		}
	}
	if allow {
		schedule.PluginsMapLock.Lock()
		defer schedule.PluginsMapLock.Unlock()
		// produce schedulers
		for str, storeIDs := range storeMap {
			schedule.PluginsMap[str].UpdateStoreIDs(cluster)
			s := strings.Split(str, "-")
			if s[0] == "Leader" {
				name := "move-leader-use-scheduler-" + s[1]
				schedules = append(schedules,
					newMoveLeaderUserScheduler(opController, name,
						schedule.PluginsMap[str].GetKeyStart(), schedule.PluginsMap[str].GetKeyEnd(), storeIDs, intervalMaps[str]))
			} else {
				name := "move-region-use-scheduler-" + s[1]
				schedules = append(schedules,
					newMoveRegionUserScheduler(opController, name,
						schedule.PluginsMap[str].GetKeyStart(), schedule.PluginsMap[str].GetKeyEnd(), storeIDs, intervalMaps[str]))
			}
		}
	}else {
		schedule.PluginsMapLock.Lock()
		defer schedule.PluginsMapLock.Unlock()
		schedule.PluginsMap = make(map[string]*schedule.PluginInfo)
	}
	return schedules
}

// IfOverlap(first, second) check wheher first and second intersect
func IfOverlap(first, second []uint64) []uint64 {
	ret := []uint64{}
	for _, i := range first {
		for _, j := range second {
			if i == j {
				ret = append(ret, i)
			}
		}
	}
	return ret
}
