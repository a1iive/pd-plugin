package admin_config

import (
	log "github.com/pingcap/log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/BurntSushi/toml"
)

type adminConfig struct {
	cfgLock *sync.RWMutex
	cfg     *dispatchConfig
}

type dispatchConfig struct {
	Enable bool
	//region regionDistribution
	Leader LeaderDistribution
}

type regionDistribution struct {
	KeyStart string
	KeyEnd   string
	StoreId  []int
}

type LeaderDistribution struct {
	RegionID int
	TargetID int
}

func NewAdminConfig() *adminConfig {
	filePath, err := filepath.Abs("dispatch_config.toml")
	if err != nil {
		log.Info("open admin config file failed")
		return nil
	}
	cfg := new(dispatchConfig)
	if _, err := toml.DecodeFile(filePath, cfg); err != nil {
		log.Info("parse admin config failed")
		return nil
	}
	return &adminConfig{
		cfgLock: new(sync.RWMutex),
		cfg:     cfg,
	}
}

func (ac *adminConfig) run() {
	go ac.start()
}

func (ac *adminConfig) start() {
	ac.LoadConfig()
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGUSR1)
	for {
		<-s
		ac.LoadConfig()
	}
}

func (ac *adminConfig) LoadConfig() bool {
	filePath, err := filepath.Abs("dispatch_config.toml")
	if err != nil {
		return false
	}
	cfg := new(dispatchConfig)
	if _, err := toml.DecodeFile(filePath, cfg); err != nil {
		return false
	}
	ac.cfgLock.Lock()
	defer ac.cfgLock.Unlock()
	ac.cfg = cfg
	return true
}
