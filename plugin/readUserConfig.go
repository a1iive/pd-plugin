package main

import (
	"github.com/pingcap/pd/server/schedule"
	"strings"
)

func ProduceScheduler(cfg schedule.Config, opController *schedule.OperatorController, cluster schedule.Cluster) []schedule.Scheduler {
	storeMap := cfg.GetStoreId(cluster)
	regionMap := cfg.GetRegionId(cluster)
	intervalMaps := cfg.GetInterval()
	var schedules []schedule.Scheduler
	for str, storeIDs := range storeMap {
		s := strings.Split(str, "-")
		if s[0] == "Leader" {
			name := "move-leader-use-scheduler-" + s[1]
			schedules = append(schedules, newMoveLeaderUserScheduler(opController, name, regionMap[str], storeIDs, intervalMaps[str]))
		}else {
			name := "move-region-use-scheduler-" + s[1]
			schedules = append(schedules, newMoveRegionUserScheduler(opController, name, regionMap[str], storeIDs, intervalMaps[str]))
		}
	}
	return schedules
}
