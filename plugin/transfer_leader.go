package schedulers

import (
	"strconv"

	"github.com/pingcap/pd/server/core"
	"github.com/pingcap/pd/server/schedule"
	"github.com/pkg/errors"
)

func init() {
	schedule.RegisterScheduler("transfer-leader", func(opController *schedule.OperatorController, args []string) (schedule.Scheduler, error) {
		if len(args) != 2 {
			return nil, errors.New("transfer-leader needs 2 argument")
		}
		regionID, err := strconv.ParseUint(args[0], 10, 64)
		storeID, err := strconv.ParseUint(args[1], 10, 64)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return newTransferLeaderScheduler(opController, regionID, storeID), nil
	})
}

type transferLeaderScheduler struct {
	*baseScheduler
	selector     *schedule.BalanceSelector
	opController *schedule.OperatorController
	regionID     uint64
	storeID      uint64
}

func newTransferLeaderScheduler(opController *schedule.OperatorController, regionID uint64, storeID uint64) schedule.Scheduler {
	filters := []schedule.Filter{
		schedule.StoreStateFilter{TransferLeader: true},
	}
	base := newBaseScheduler(opController)
	s := &transferLeaderScheduler{
		baseScheduler: base,
		selector:      schedule.NewBalanceSelector(core.LeaderKind, filters),
		opController:  opController,
		regionID:      regionID,
		storeID:       storeID,
	}
	return s
}

func (l *transferLeaderScheduler) GetName() string {
	return "transfer-leader-scheduler"
}

func (l *transferLeaderScheduler) GetType() string {
	return "transfer-leader"
}

func (l *transferLeaderScheduler) IsScheduleAllowed(cluster schedule.Cluster) bool {
	return l.opController.OperatorCount(schedule.OpLeader) < cluster.GetLeaderScheduleLimit()
}

func (l *transferLeaderScheduler) Schedule(cluster schedule.Cluster) []*schedule.Operator {
	schedulerCounter.WithLabelValues(l.GetName(), "schedule").Inc()
	// 选择分高的target store
	//stores := cluster.GetStores()
	//target := l.selector.SelectTarget(cluster, stores)

	//根据label选择store
	//for _ , store := range cluster.GetStores(){
	//	store.GetLabels()  ??
	//}

	target := cluster.GetStore(l.storeID)
	if target == nil {
		schedulerCounter.WithLabelValues(l.GetName(), "no_target_store").Inc()
		return nil
	}
	region := cluster.GetRegion(l.regionID)
	if region == nil {
		schedulerCounter.WithLabelValues(l.GetName(), "no_region").Inc()
		return nil
	}
	sourceID := cluster.GetRegion(l.regionID).GetLeader().GetStoreId()
	targetID := target.GetID()

	if sourceID != targetID {
		schedulerCounter.WithLabelValues(l.GetName(), "new_operator").Inc()
		op := schedule.CreateTransferLeaderOperator("transfer-leader", region, sourceID, targetID, schedule.OpAdmin)
		op.SetPriorityLevel(core.HighPriority)
		return []*schedule.Operator{op}

	} else {
		return nil
	}

}
