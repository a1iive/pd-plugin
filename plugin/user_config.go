package main

import (
	"github.com/pingcap/pd/server/schedule"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/pingcap/log"
	"go.uber.org/zap"
)

type userConfig struct {
	cfgLock sync.RWMutex
	version uint64
	cfg     *dispatchConfig
}

type dispatchConfig struct {
	Leaders leaders
	Regions regions
}

type leaders struct {
	Leader []moveLeader
}

type regions struct {
	Region []moveRegion
}

type moveLeader struct {
	Persist   bool
	KeyStart  string
	KeyEnd    string
	Stores    []schedule.StoreLabels
	StartTime time.Time
	EndTime   time.Time
}

type moveRegion struct {
	Persist   bool
	KeyStart  string
	KeyEnd    string
	Stores    []schedule.StoreLabels
	StartTime time.Time
	EndTime   time.Time
}

func NewUserConfig() schedule.Config {
	ret := &userConfig{
		cfgLock: sync.RWMutex{},
		version: 1,
		cfg:     nil,
	}
	return ret
}

// Load and decode config file
// if conflict, return false
// if not conflict, reset pluginMap 
func (uc *userConfig) LoadConfig(path string, maxReplicas int) bool {
	filePath, err := filepath.Abs(path)
	if err != nil {
		log.Error("open file failed", zap.Error(err))
		return false
	}
	log.Info("parse toml file once. ", zap.String("filePath", filePath))
	cfg := new(dispatchConfig)
	if _, err := toml.DecodeFile(filePath, cfg); err != nil {
		log.Error("parse user config failed", zap.Error(err))
		return false
	}
	uc.cfgLock.Lock()
	defer uc.cfgLock.Unlock()
	schedule.PluginsMapLock.Lock()
	defer schedule.PluginsMapLock.Unlock()
	uc.cfg = cfg
	if uc.cfg != nil && uc.IfConflict(maxReplicas) {
		return false
	}
	schedule.PluginsMap = make(map[string]*schedule.PluginInfo)
	for i, info := range uc.cfg.Leaders.Leader {
		pi := &schedule.PluginInfo{
			Persist:  info.Persist,
			KeyStart: info.KeyStart,
			KeyEnd:   info.KeyEnd,
			Interval: &schedule.TimeInterval{Begin: info.StartTime, End: info.EndTime},
			Stores:   info.Stores,
			StoreIDs: []uint64{},
		}
		str := "Leader-" + strconv.Itoa(i)
		schedule.PluginsMap[str] = pi
	}

	for i, info := range uc.cfg.Regions.Region {
		pi := &schedule.PluginInfo{
			Persist:  info.Persist,
			KeyStart: info.KeyStart,
			KeyEnd:   info.KeyEnd,
			Interval: &schedule.TimeInterval{Begin: info.StartTime, End: info.EndTime},
			Stores:   info.Stores,
			StoreIDs: []uint64{},
		}
		str := "Region-" + strconv.Itoa(i)
		schedule.PluginsMap[str] = pi
	}

	uc.version++
	return true
}

func (uc *userConfig) GetStoreId(cluster schedule.Cluster) map[string][]uint64 {
	ret := make(map[string][]uint64)
	for i, Leader := range uc.cfg.Leaders.Leader {
		for _, s := range Leader.Stores {
			if store := schedule.GetStoreByLabel(cluster, s.StoreLabel); store != nil {
				str := "Leader-" + strconv.Itoa(i)
				log.Info(str, zap.Uint64("store-id", store.GetID()))
				ret[str] = append(ret[str], store.GetID())
			}
		}
	}
	for i, Region := range uc.cfg.Regions.Region {
		for _, s := range Region.Stores {
			if store := schedule.GetStoreByLabel(cluster, s.StoreLabel); store != nil {
				str := "Region-" + strconv.Itoa(i)
				log.Info(str, zap.Uint64("store-id", store.GetID()))
				ret[str] = append(ret[str], store.GetID())
			}
		}
	}
	return ret
}

func (uc *userConfig) GetInterval() map[string]*schedule.TimeInterval {
	ret := make(map[string]*schedule.TimeInterval)
	for i, Leader := range uc.cfg.Leaders.Leader {
		str := "Leader-" + strconv.Itoa(i)
		interval := &schedule.TimeInterval{
			Begin: Leader.StartTime,
			End:   Leader.EndTime,
		}
		ret[str] = interval
	}
	for i, Region := range uc.cfg.Regions.Region {
		str := "Region-" + strconv.Itoa(i)
		interval := &schedule.TimeInterval{
			Begin: Region.StartTime,
			End:   Region.EndTime,
		}
		ret[str] = interval
	}
	return ret
}

// Check if there are conflicts in similar type of rules
// eg. move-leader&move-leader or move-region&move-region
func (uc *userConfig) IfConflict(maxReplicas int) bool {
	ret := false
	// move_leaders
	for i, l1 := range uc.cfg.Leaders.Leader {
		for j, l2 := range uc.cfg.Leaders.Leader {
			if i < j {
				if (l1.KeyStart <= l2.KeyStart && l1.KeyEnd > l2.KeyStart) ||
					(l2.KeyStart <= l1.KeyStart && l2.KeyEnd > l1.KeyStart) {
					if ((l1.StartTime.Before(l2.StartTime) || l1.StartTime.Equal(l2.StartTime)) && 
							l1.EndTime.After(l2.StartTime)) || 
						((l2.StartTime.Before(l1.StartTime) || l2.StartTime.Equal(l1.StartTime)) && 
							l2.EndTime.After(l1.StartTime)) {
						log.Error("Key Range Conflict", zap.Ints("Config Move-Leader Nums", []int{i, j}))
						ret = true
					}

				}
			}
		}
	}
	// move_regions
	for i, r1 := range uc.cfg.Regions.Region {
		for j, r2 := range uc.cfg.Regions.Region {
			if i < j {
				if (r1.KeyStart <= r2.KeyStart && r1.KeyEnd > r2.KeyStart) ||
					(r2.KeyStart <= r1.KeyStart && r2.KeyEnd > r1.KeyStart) {
					if ((r1.StartTime.Before(r2.StartTime) || r1.StartTime.Equal(r2.StartTime)) &&
							r1.EndTime.After(r2.StartTime)) ||
						((r2.StartTime.Before(r1.StartTime) || r2.StartTime.Equal(r1.StartTime)) &&
							r2.EndTime.After(r1.StartTime)) {
						log.Error("Key Range Conflict", zap.Ints("Config Move-Region Nums", []int{i, j}))
						ret = true
					}
				}
			}
		}
	}
	// store nums > max replicas
	for i, r := range uc.cfg.Regions.Region {
		if len(r.Stores) > maxReplicas {
			log.Error("the number of stores is beyond the max replicas", zap.Int("Config Move-Region Nums", i))
			ret = true
		}
	}
	return ret
}

// Check if there are conflicts in different type of rules
// if conflicts exist, return ids groups to do further judgment
func (uc *userConfig) IfNeedCheckStore() [][]int {
	ret := [][]int{}
	for i, l := range uc.cfg.Leaders.Leader {
		for j, r := range uc.cfg.Regions.Region {
			if (l.KeyStart <= r.KeyStart && l.KeyEnd > r.KeyStart) ||
				(r.KeyStart <= l.KeyStart && r.KeyEnd > l.KeyStart) {
				if ((l.StartTime.Before(r.StartTime) || l.StartTime.Equal(r.StartTime)) &&
						l.EndTime.After(r.StartTime)) ||
					((r.StartTime.Before(l.StartTime) || r.StartTime.Equal(l.StartTime)) &&
						r.EndTime.After(l.StartTime)) {
					ret = append(ret, []int{i, j})
				}
			}
		}
	}
	return ret
}
