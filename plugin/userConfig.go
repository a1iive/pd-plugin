package main

import (
	"encoding/hex"
	"github.com/pingcap/pd/server/core"
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
	Stores    []stores
	StartTime time.Time
	EndTime   time.Time
}

type moveRegion struct {
	Persist   bool
	KeyStart  string
	KeyEnd    string
	Stores    []stores
	StartTime time.Time
	EndTime   time.Time
}

type stores struct {
	StoreLabel []schedule.Label
}

func NewUserConfig() schedule.Config {
	ret := &userConfig{
		cfgLock: sync.RWMutex{},
		version: 1,
		cfg:     nil,
	}
	ret.LoadConfig()
	return ret
}

func (uc *userConfig) LoadConfig() {
	filePath, err := filepath.Abs("./conf/user_config.toml")
	if err != nil {
		log.Error("open file failed", zap.Error(err))
	}
	log.Info("parse toml file once. ", zap.String("filePath", filePath))
	cfg := new(dispatchConfig)
	if _, err := toml.DecodeFile(filePath, cfg); err != nil {
		log.Error("parse user config failed", zap.Error(err))
	}
	uc.cfgLock.Lock()
	defer uc.cfgLock.Unlock()
	schedule.PluginsMapLock.Lock()
	defer schedule.PluginsMapLock.Unlock()
	uc.cfg = cfg
	uc.version++

	for i, info := range uc.cfg.Leaders.Leader {
		pi := &schedule.PluginInfo{
			Persist:   info.Persist,
			KeyStart:  info.KeyStart,
			KeyEnd:    info.KeyEnd,
			Interval:  &schedule.TimeInterval{Begin: info.StartTime, End: info.EndTime},
			RegionIDs: []uint64{},
			StoreIDs:  []uint64{},
		}
		str := "Leader-" + strconv.Itoa(i)
		schedule.PluginsMap[str] = pi
	}

	for i, info := range uc.cfg.Regions.Region {
		pi := &schedule.PluginInfo{
			Persist:   info.Persist,
			KeyStart:  info.KeyStart,
			KeyEnd:    info.KeyEnd,
			Interval:  &schedule.TimeInterval{Begin: info.StartTime, End: info.EndTime},
			RegionIDs: []uint64{},
			StoreIDs:  []uint64{},
		}
		str := "Region-" + strconv.Itoa(i)
		schedule.PluginsMap[str] = pi
	}

}

func (uc *userConfig) GetStoreId(cluster schedule.Cluster) map[string][]uint64 {
	schedule.PluginsMapLock.Lock()
	defer schedule.PluginsMapLock.Unlock()

	ret := make(map[string][]uint64)
	for i, Leader := range uc.cfg.Leaders.Leader {
		for _, s := range Leader.Stores {
			if store := uc.GetStoreByLabel(cluster, s.StoreLabel); store != nil {
				str := "Leader-" + strconv.Itoa(i)
				schedule.PluginsMap[str].StoreIDs = append(schedule.PluginsMap[str].StoreIDs, store.GetID())
				log.Info("GetStoreId-MoveLeader", zap.Uint64("store-id", store.GetID()))
				ret[str] = append(ret[str], store.GetID())
			}
		}
	}
	for i, Region := range uc.cfg.Regions.Region {
		for _, s := range Region.Stores {
			if store := uc.GetStoreByLabel(cluster, s.StoreLabel); store != nil {
				str := "Region-" + strconv.Itoa(i)
				schedule.PluginsMap[str].StoreIDs = append(schedule.PluginsMap[str].StoreIDs, store.GetID())
				log.Info("GetStoreId-MoveRegion", zap.Uint64("store-id", store.GetID()))
				ret[str] = append(ret[str], store.GetID())
			}
		}
	}
	return ret
}

func (uc *userConfig) GetRegionId(cluster schedule.Cluster) map[string][]uint64 {
	schedule.PluginsMapLock.Lock()
	defer schedule.PluginsMapLock.Unlock()

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

		regions := cluster.ScanRangeWithEndKey(startKey, endKey)
		for _, region := range regions {
			log.Info("GetRegionId-MoveLeader", zap.Uint64("region-id", region.GetID()))
			ret[str] = append(ret[str], region.GetID())
			schedule.PluginsMap[str].RegionIDs = append(schedule.PluginsMap[str].RegionIDs, region.GetID())
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

		regions := cluster.ScanRangeWithEndKey(startKey, endKey)
		for _, region := range regions {
			log.Info("GetRegionId-MoveRegion", zap.Uint64("region-id", region.GetID()))
			ret[str] = append(ret[str], region.GetID())
			schedule.PluginsMap[str].RegionIDs = append(schedule.PluginsMap[str].RegionIDs, region.GetID())
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

func (uc *userConfig) GetStoreByLabel(cluster schedule.Cluster, storeLabel []schedule.Label) *core.StoreInfo {
	length := len(storeLabel)
	for _, store := range cluster.GetStores() {
		sum := 0
		storeLabels := store.GetMeta().Labels
		for _, label := range storeLabels {
			for _, myLabel := range storeLabel {
				if myLabel.Key == label.Key && myLabel.Value == label.Value {
					sum++
					log.Info("GetStoreIdLeader match", zap.String(myLabel.Key, myLabel.Value))
					continue
				}
			}
		}
		if sum == length {
			return store
		}
	}
	return nil
}
