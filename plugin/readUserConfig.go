package main

import (
	"github.com/pingcap/pd/server/schedule"
)

func ProduceScheduler(cfg schedule.Config, opController *schedule.OperatorController, cluster schedule.Cluster) []schedule.Scheduler {
	storeIDs := cfg.GetStoreId(cluster)
	regionIDs := cfg.GetRegionId(cluster)
	interval := cfg.GetInterval()
	var schedules []schedule.Scheduler
	//schedules = append(schedules, newMoveLeaderUserScheduler(opController, regionIDs["Leader"], storeIDs["Leader"], interval["Leader"]))
	schedules = append(schedules, newMoveRegionUserScheduler(opController, regionIDs["Region"], storeIDs["Region"], interval["Region"]))
	return schedules
}
