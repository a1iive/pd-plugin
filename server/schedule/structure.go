package schedule

import (
	"encoding/hex"
	"github.com/pingcap/pd/server/core"
	"path/filepath"
	"plugin"
	"sync"
	"time"

	"github.com/pingcap/log"
	"go.uber.org/zap"
)

type PluginInfo struct {
	Persist   bool
	KeyStart  string
	KeyEnd    string
	Interval  *TimeInterval
	Stores    []StoreLabels
	RegionIDs []uint64
	StoreIDs  []uint64
}

func (p *PluginInfo) GetInterval() *TimeInterval {
	return p.Interval
}

func (p *PluginInfo) GetRegionIDs() []uint64 {
	return p.RegionIDs
}

func (p *PluginInfo) GetStoreIDs() []uint64 {
	return p.StoreIDs
}

func (p *PluginInfo) UpdateRegionIDs(cluster Cluster) {
	p.RegionIDs = []uint64{}
	//decode key form string to []byte
	startKey, err := hex.DecodeString(p.KeyStart)
	if err != nil {
		log.Error("can not decode", zap.String("key:", p.KeyStart))
	}
	endKey, err := hex.DecodeString(p.KeyEnd)
	if err != nil {
		log.Info("can not decode", zap.String("key:", p.KeyEnd))
	}

	lastKey := []byte{}
	regions := cluster.ScanRangeWithEndKey(startKey, endKey)
	for _, region := range regions {
		p.RegionIDs = append(p.RegionIDs, region.GetID())
		lastKey = region.GetEndKey()
	}
	lastRegion := cluster.ScanRegions(lastKey, 1)
	if len(lastRegion) != 0 {
		p.RegionIDs = append(p.RegionIDs, lastRegion[0].GetID())
	}
}

func (p *PluginInfo) UpdateStoreIDs(cluster Cluster) {
	p.StoreIDs = []uint64{}
	for _, s := range p.Stores {
		if store := GetStoreByLabel(cluster, s.StoreLabel); store != nil {
			p.StoreIDs = append(p.StoreIDs, store.GetID())
		}
	}
}

type Label struct {
	Key   string
	Value string
}

type StoreLabels struct {
	StoreLabel []Label
}

type TimeInterval struct {
	Begin time.Time
	End   time.Time
}

var PluginsMapLock = sync.RWMutex{}
var PluginsMap = make(map[string]*PluginInfo)

var PluginLock = sync.RWMutex{}
var Plugin *plugin.Plugin = nil

func GetFunction(path string, funcName string) (plugin.Symbol, error) {
	PluginLock.Lock()
	if Plugin == nil {
		//open plugin
		filePath, err := filepath.Abs(path)
		if err != nil {
			PluginLock.Unlock()
			return nil, err
		}
		log.Info("open plugin file", zap.String("file-path", filePath))
		p, err := plugin.Open(filePath)
		if err != nil {
			PluginLock.Unlock()
			return nil, err
		}
		Plugin = p
	}
	PluginLock.Unlock()
	PluginLock.RLock()
	defer PluginLock.RUnlock()
	//get func from plugin
	//func : NewUserConfig()
	f, err := Plugin.Lookup(funcName)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func GetStoreByLabel(cluster Cluster, storeLabel []Label) *core.StoreInfo {
	length := len(storeLabel)
	for _, store := range cluster.GetStores() {
		sum := 0
		storeLabels := store.GetMeta().Labels
		for _, label := range storeLabels {
			for _, myLabel := range storeLabel {
				if myLabel.Key == label.Key && myLabel.Value == label.Value {
					sum++
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
