package main

import (
	"github.com/pingcap/pd/server/schedule"
)

func ProduceScheduler(cfg schedule.Config, opController *schedule.OperatorController, cluster schedule.Cluster) []schedule.Scheduler {
	storeIDs := cfg.GetStoreId(cluster)
	regionIDs := cfg.GetRegionId(cluster)
	interval := cfg.GetInterval()
	var schedules []schedule.Scheduler
	schedules = append(schedules, newMoveLeaderUserScheduler(opController, regionIDs["Leader"], storeIDs["Leader"], interval["Leader"]))
	schedules = append(schedules, newMoveRegionUserScheduler(opController, regionIDs["Region"], storeIDs["Region"], interval["Region"]))
	return schedules
}

func ProduceOperator(cfg schedule.Config, opController *schedule.OperatorController, cluster schedule.Cluster) []*schedule.Operator {
	storeIds := cfg.GetStoreId(cluster)
	regionIds := cfg.GetRegionId(cluster)
	var operators []*schedule.Operator
	newMoveLeaderUserScheduler(opController, regionIds["Leader"], storeIds["Leader"], nil)
	operators = append(operators, userScheduler1.Schedule(cluster)...)
	operators = append(operators, userScheduler2.Schedule(cluster)...)
	return operators
}
