package main

import (
	"encoding/hex"
	"github.com/pingcap/pd/server/schedule"
	"path/filepath"
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
	Leader moveLeader
	Region moveRegion
}

type moveLeader struct {
	Persist   bool
	KeyStart  string
	KeyEnd    string
	Stores    []stores
	StartTime time.Time
	EndTime   time.Time
	HighPerf  bool
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

	pi := &schedule.PluginInfo{
		KeyStart:  uc.cfg.Leader.KeyStart,
		KeyEnd:    uc.cfg.Leader.KeyEnd,
		Interval:  schedule.TimeInterval{Begin: uc.cfg.Leader.StartTime, End: uc.cfg.Leader.EndTime},
		RegionIDs: []uint64{},
		StoreIDs:  []uint64{},
	}
	schedule.PluginsMap["Leader"] = pi
	pi2 := &schedule.PluginInfo{
		KeyStart:  uc.cfg.Region.KeyStart,
		KeyEnd:    uc.cfg.Region.KeyEnd,
		Interval:  schedule.TimeInterval{Begin: uc.cfg.Region.StartTime, End: uc.cfg.Region.EndTime},
		RegionIDs: []uint64{},
		StoreIDs:  []uint64{},
	}
	schedule.PluginsMap["Region"] = pi2
}

func (uc *userConfig) GetInterval() map[string]*schedule.TimeInterval{
	ret := make(map[string]*schedule.TimeInterval)
	ret["Leader"] =  &schedule.TimeInterval{
		Begin: uc.cfg.Leader.StartTime,
		End:   uc.cfg.Leader.EndTime,
	}
	ret["Region"] = &schedule.TimeInterval{
		Begin: uc.cfg.Region.StartTime,
		End:   uc.cfg.Region.EndTime,
	}
	return ret
}

func (uc *userConfig) GetStoreId(cluster schedule.Cluster) map[string][]uint64 {
	ret := make(map[string][]uint64)
	ret["Leader"] = uc.GetStoreIdLeader(cluster)
	ret["Region"] = uc.GetStoreIdRegion(cluster)
	return ret
}

func (uc *userConfig) GetRegionId(cluster schedule.Cluster) map[string][]uint64 {
	ret := make(map[string][]uint64)
	ret["Leader"] = uc.GetRegionIdLeader(cluster)
	ret["Region"] = uc.GetRegionIdRegion(cluster)
	return ret
}

func (uc *userConfig) GetStoreIdLeader(cluster schedule.Cluster) []uint64 {
	for _, s := range uc.cfg.Leader.Stores {
		lLen := len(s.StoreLabel)
		for _, store := range cluster.GetStores() {
			sum := 0
			storeLabels := store.GetMeta().Labels
			for _, label := range storeLabels {
				for _, myLabel := range s.StoreLabel {
					if myLabel.Key == label.Key && myLabel.Value == label.Value {
						sum++
						log.Info("GetStoreIdLeader match", zap.String(myLabel.Key, myLabel.Value))
						continue
					}
				}
			}
			if sum == lLen {
				pi := schedule.PluginsMap["Leader"]
				pi.StoreIDs = append(pi.StoreIDs, store.GetID())
				schedule.PluginsMap["Leader"] = pi
				log.Info("GetStoreIdLeader", zap.Uint64("store-id", store.GetID()))
				return []uint64{store.GetID()}
			}
		}
	}
	return []uint64{0}
}

func (uc *userConfig) GetRegionIdLeader(cluster schedule.Cluster) []uint64 {
	schedule.PluginsMapLock.Lock()
	defer schedule.PluginsMapLock.Unlock()

	var ret []uint64

	startKey, err := hex.DecodeString(uc.cfg.Leader.KeyStart)
	if err != nil {
		log.Info("Bad format region start key", zap.String("key", uc.cfg.Leader.KeyStart))
		return ret
	}

	endKey, err := hex.DecodeString(uc.cfg.Leader.KeyEnd)
	if err != nil {
		log.Info("Bad format region end key", zap.String("key", uc.cfg.Leader.KeyEnd))
		return ret
	}

	regions := cluster.ScanRangeWithEndKey(startKey, endKey)
	for _, region := range regions {
		log.Info("GetRegionIdLeader", zap.Uint64("region-id", region.GetID()))
		ret = append(ret, region.GetID())
	}

	pi := schedule.PluginsMap["Leader"]
	pi.RegionIDs = append(pi.RegionIDs, ret...)
	schedule.PluginsMap["Leader"] = pi

	return ret
}

func (uc *userConfig) GetStoreIdRegion(cluster schedule.Cluster) []uint64 {
	for _, s := range uc.cfg.Region.Stores {
		lLen := len(s.StoreLabel)
		for _, store := range cluster.GetStores() {
			sum := 0
			storeLabels := store.GetMeta().Labels
			for _, label := range storeLabels {
				for _, myLabel := range s.StoreLabel {
					if myLabel.Key == label.Key && myLabel.Value == label.Value {
						sum++
						log.Info("GetStoreIdRegion match", zap.String(myLabel.Key, myLabel.Value))
						continue
					}
				}
			}
			if sum == lLen {
				pi := schedule.PluginsMap["Region"]
				pi.StoreIDs = append(pi.StoreIDs, store.GetID())
				schedule.PluginsMap["Region"] = pi
				log.Info("GetStoreIdRegion", zap.Uint64("store-id", store.GetID()))
				return []uint64{store.GetID()}
			}
		}
	}
	return []uint64{0}
}

func (uc *userConfig) GetRegionIdRegion(cluster schedule.Cluster) []uint64 {
	schedule.PluginsMapLock.Lock()
	defer schedule.PluginsMapLock.Unlock()

	var ret []uint64

	log.Info("start key", zap.String("key", uc.cfg.Region.KeyStart))
	startKey, err := hex.DecodeString(uc.cfg.Region.KeyStart)
	if err != nil {
		log.Info("Bad format region start key", zap.String("key", uc.cfg.Region.KeyStart))
		return ret
	}
	log.Info("decode start key", zap.ByteString("key", startKey))

	log.Info("end key", zap.String("key", uc.cfg.Region.KeyEnd))
	endKey, err := hex.DecodeString(uc.cfg.Region.KeyEnd)
	if err != nil {
		log.Info("Bad format region end key", zap.String("key", uc.cfg.Region.KeyEnd))
		return ret
	}
	log.Info("decode end key", zap.ByteString("key", endKey))

	regions := cluster.ScanRangeWithEndKey(startKey, endKey)
	for _, region := range regions {
		log.Info("GetRegionIdRegion", zap.Uint64("region-id", region.GetID()))
		ret = append(ret, region.GetID())
	}

	log.Info("GetRegionIdRegion end")
	pi := schedule.PluginsMap["Region"]
	pi.RegionIDs = append(pi.RegionIDs, ret...)
	schedule.PluginsMap["Region"] = pi

	return ret
}
