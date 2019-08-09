package main

import (
	"encoding/hex"
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

func (uc *userConfig) LoadConfig() bool {
	filePath, err := filepath.Abs("./conf/user_config.toml")
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
	if uc.cfg != nil && uc.IfConflict() {
		return false
	}

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

func (uc *userConfig) GetRegionId(cluster schedule.Cluster) map[string][]uint64 {
	ret := make(map[string][]uint64)

	for i, Leader := range uc.cfg.Leaders.Leader {
		str := "Leader-" + strconv.Itoa(i)
		//decode key form string to []byte
		startKey, err := hex.DecodeString(Leader.KeyStart)
		if err != nil {
			log.Info("can not decode", zap.String("key:", Leader.KeyStart))
			continue
		}
		endKey, err := hex.DecodeString(Leader.KeyEnd)
		if err != nil {
			log.Info("can not decode", zap.String("key:", Leader.KeyEnd))
			continue
		}

		lastKey := []byte{}
		regions := cluster.ScanRangeWithEndKey(startKey, endKey)
		for _, region := range regions {
			log.Info(str, zap.Uint64("region-id", region.GetID()))
			ret[str] = append(ret[str], region.GetID())
			lastKey = region.GetEndKey()
		}
		lastRegion := cluster.ScanRegions(lastKey, 1)
		if len(lastRegion) != 0 {
			log.Info(str, zap.Uint64("region-id", lastRegion[0].GetID()))
			ret[str] = append(ret[str], lastRegion[0].GetID())
		}
	}

	for i, Region := range uc.cfg.Regions.Region {
		str := "Region-" + strconv.Itoa(i)
		//decode key form string to []byte
		startKey, err := hex.DecodeString(Region.KeyStart)
		if err != nil {
			log.Info("can not decode", zap.String("key:", Region.KeyStart))
			continue
		}
		endKey, err := hex.DecodeString(Region.KeyEnd)
		if err != nil {
			log.Info("can not decode", zap.String("key:", Region.KeyEnd))
			continue
		}

		lastKey := []byte{}
		regions := cluster.ScanRangeWithEndKey(startKey, endKey)
		for _, region := range regions {
			log.Info(str, zap.Uint64("region-id", region.GetID()))
			ret[str] = append(ret[str], region.GetID())
			lastKey = region.GetEndKey()
		}
		lastRegion := cluster.ScanRegions(lastKey, 1)
		if len(lastRegion) != 0 {
			log.Info(str, zap.Uint64("region-id", lastRegion[0].GetID()))
			ret[str] = append(ret[str], lastRegion[0].GetID())
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

func (uc *userConfig) IfConflict() bool {
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
	return ret
}

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
