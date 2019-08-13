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
	Persist  bool
	KeyStart string
	KeyEnd   string
	Interval *TimeInterval
	Stores   []StoreLabels
	StoreIDs []uint64
}

func (p *PluginInfo) GetInterval() *TimeInterval {
	if p != nil {
		return p.Interval
	} else {
		return nil
	}

}

func (p *PluginInfo) GetStoreIDs() []uint64 {
	if p != nil {
		return p.StoreIDs
	} else {
		return []uint64{}
	}
}

func (p *PluginInfo) GetKeyStart() string {
	if p != nil {
		return p.KeyStart
	} else {
		return ""
	}
}
func (p *PluginInfo) GetKeyEnd() string {
	if p != nil {
		return p.KeyEnd
	} else {
		return ""
	}
}

func (p *PluginInfo) UpdateStoreIDs(cluster Cluster) {
	if p == nil {
		return
	}
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

func (t *TimeInterval) GetBegin() time.Time {
	if t != nil {
		return t.Begin
	} else {
		return time.Time{}
	}
}

func (t *TimeInterval) GetEnd() time.Time {
	if t != nil {
		return t.End
	} else {
		return time.Time{}
	}
}

var PluginsMapLock = sync.RWMutex{}
var PluginsMap = make(map[string]*PluginInfo)

var PluginLock = sync.RWMutex{}
var Plugin *plugin.Plugin = nil

// get func by funcName from plugin(.so)
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

func GetRegionIDs(cluster Cluster, keyStart, keyEnd string) []uint64 {
	regionIDs := []uint64{}
	//decode key form string to []byte
	startKey, err := hex.DecodeString(keyStart)
	if err != nil {
		log.Error("can not decode", zap.String("key:", keyStart))
		return regionIDs
	}
	endKey, err := hex.DecodeString(keyEnd)
	if err != nil {
		log.Info("can not decode", zap.String("key:", keyEnd))
		return regionIDs
	}

	lastKey := []byte{}
	regions := cluster.ScanRangeWithEndKey(startKey, endKey)
	for _, region := range regions {
		regionIDs = append(regionIDs, region.GetID())
		lastKey = region.GetEndKey()
	}

	if len(regions) == 0{
		lastRegion := cluster.ScanRegions(startKey, 1)
		if len(lastRegion) == 0{
			return regionIDs
		}else {
			//if get the only one region, exclude it
			if len(lastRegion[0].GetStartKey()) == 0 && len(lastRegion[0].GetEndKey()) == 0{
				return regionIDs
			}else {
				//      startKey         endKey
				//         |			   |
				//     -----------------------------
				// ...|	  region0  |    region1  | ...
				//    -----------------------------
				// key range span two regions
				// choose region1
				regionIDs = append(regionIDs, lastRegion[0].GetID())
				return regionIDs
			}
		}
	}else {
		if len(lastKey) == 0{
			// if regions last one is the last region
			return regionIDs
		}else {
			//            startKey		                            endKey
			//         	   |                                         |
			//      -----------------------------------------------------------
			// ... |	  region_i   |   ...   |     region_j   |    region_j+1   | ...
			//     -----------------------------------------------------------
			// ScanRangeWithEndKey(startKey, endKey) will get region i+1 to j
			// lastKey = region_j's EndKey
			// ScanRegions(lastKey, 1) then get region_j+1
			// so finally get region i+1 to j+1
			lastRegion := cluster.ScanRegions(lastKey, 1)
			if len(lastRegion) != 0 {
				regionIDs = append(regionIDs, lastRegion[0].GetID())
			}
			return regionIDs
		}
	}
}
