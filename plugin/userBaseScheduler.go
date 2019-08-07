package main

import (
	"github.com/pingcap/pd/server/schedule"
	"log"
	"time"
)

type userBaseScheduler struct {
	opController *schedule.OperatorController
}

// options for interval of schedulers
const (
	MaxScheduleInterval     = time.Second * 5
	MinScheduleInterval     = time.Millisecond * 10
	MinSlowScheduleInterval = time.Second * 3

	ScheduleIntervalFactor = 1.3
)

type intervalGrowthType int

const (
	exponentailGrowth intervalGrowthType = iota
	linearGrowth
	zeroGrowth
)

// intervalGrow calculates the next interval of balance.
func intervalGrow(x time.Duration, maxInterval time.Duration, typ intervalGrowthType) time.Duration {
	switch typ {
	case exponentailGrowth:
		return minDuration(time.Duration(float64(x)*ScheduleIntervalFactor), maxInterval)
	case linearGrowth:
		return minDuration(x+MinSlowScheduleInterval, maxInterval)
	case zeroGrowth:
		return x
	default:
		log.Fatal("unknown interval growth type")
	}
	return 0
}

func newUserBaseScheduler(opController *schedule.OperatorController) *userBaseScheduler {
	return &userBaseScheduler{opController: opController}
}

func (s *userBaseScheduler) Prepare(cluster schedule.Cluster) error { return nil }

func (s *userBaseScheduler) Cleanup(cluster schedule.Cluster) {}

func (s *userBaseScheduler) GetMinInterval() time.Duration {
	return MinScheduleInterval
}

func (s *userBaseScheduler) GetNextInterval(interval time.Duration) time.Duration {
	return intervalGrow(interval, MaxScheduleInterval, exponentailGrowth)
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
