package schedule

import (
	"path/filepath"
	"plugin"
	"sync"
	"time"

	"github.com/pingcap/log"
	"go.uber.org/zap"
)

type PluginInfo struct {
	KeyStart  string
	KeyEnd    string
	Interval  TimeInterval
	RegionIDs []uint64
	StoreIDs  []uint64
}

func (p *PluginInfo) GetInterval() *TimeInterval {
	return &p.Interval
}

func (p *PluginInfo) GetRegionIDs() []uint64 {
	return p.RegionIDs
}

func (p *PluginInfo) GetStoreIDs() []uint64 {
	return p.StoreIDs
}

type Label struct {
	Key   string
	Value string
}

type TimeInterval struct {
	Begin time.Time
	End   time.Time
}

var PluginsMapLock = sync.RWMutex{}
var PluginsMap = make(map[string]*PluginInfo)

var PluginLock = sync.RWMutex{}
var Plugin *plugin.Plugin= nil

func GetFunction(path string, funcName string) (plugin.Symbol, error){
	PluginLock.Lock()
	if Plugin == nil{
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
	log.Info("plugin unlock")
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
