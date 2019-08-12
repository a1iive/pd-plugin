package main

import (
	"bufio"
	. "github.com/pingcap/check"
	"github.com/pingcap/pd/pkg/mock/mockcluster"
	_ "github.com/pingcap/pd/pkg/mock/mockcluster"
	"github.com/pingcap/pd/pkg/mock/mockhbstream"
	"github.com/pingcap/pd/pkg/mock/mockoption"
	"github.com/pingcap/pd/server/schedule"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var _ = Suite(&testUserConfigSuite{})

type testUserConfigSuite struct {
}

func (s *testUserConfigSuite) TestReadUserConfig(c *C) {
	filePath, err := filepath.Abs("../conf/test_config.toml")
	c.Assert(err, IsNil)
	uc := NewUserConfig()
	c.Assert(uc.LoadConfig(filePath), Equals, true)
	schedule.PluginsMapLock.RLock()
	defer schedule.PluginsMapLock.RUnlock()
	f, err := os.Open(filePath)
	c.Assert(err, IsNil)
	defer f.Close()
	br := bufio.NewReader(f)
	var line string
	names := []string{"Leader-0", "Leader-1", "Region-0", "Region-1"}
	for _, name := range names {
		pluginInfo := schedule.PluginsMap[name]
		line, err = br.ReadString('\n')
		if strings.Contains(line, "[[") == false {
			line, err = br.ReadString('\n')
		}
		line, err = br.ReadString('\n')
		c.Assert(strings.Contains(line, strconv.FormatBool(pluginInfo.Persist)), Equals, true)
		line, err = br.ReadString('\n')
		c.Assert(strings.Contains(line, pluginInfo.GetKeyStart()), Equals, true)
		line, err = br.ReadString('\n')
		c.Assert(strings.Contains(line, pluginInfo.GetKeyEnd()), Equals, true)
		line, err = br.ReadString('"')
		line, err = br.ReadString('+')
		line = strings.Replace(line, "+", "", -1)
		line = strings.Replace(line, "T", " ", -1)
		t, _ := time.ParseInLocation("2006-01-02 15:04:05", line, time.Local)
		line, err = br.ReadString('\n')
		c.Assert(pluginInfo.Interval.Begin.Format("2006-01-02 15:04:05"), Equals, t.Format("2006-01-02 15:04:05"))
		line, err = br.ReadString('"')
		line, err = br.ReadString('+')
		line = strings.Replace(line, "+", "", -1)
		line = strings.Replace(line, "T", " ", -1)
		t, _ = time.ParseInLocation("2006-01-02 15:04:05", line, time.Local)
		line, err = br.ReadString('\n')
		c.Assert(pluginInfo.Interval.End.Format("2006-01-02 15:04:05"), Equals, t.Format("2006-01-02 15:04:05"))

		for _, store := range pluginInfo.Stores {
			line, err = br.ReadString('\n')
			for _, label := range store.StoreLabel {
				line, err = br.ReadString('\n')
				line, err = br.ReadString('\n')
				c.Assert(strings.Contains(line, label.Key), Equals, true)
				line, err = br.ReadString('\n')
				c.Assert(strings.Contains(line, label.Value), Equals, true)
			}
		}
	}
}

func (s *testUserConfigSuite) TestProduceScheduler(c *C) {
	opt := mockoption.NewScheduleOptions()
	cluster := mockcluster.NewCluster(opt)
	htStream := mockhbstream.NewHeartbeatStream()
	opc := schedule.NewOperatorController(cluster, htStream)
	filePath, err := filepath.Abs("../conf/test_config.toml")
	c.Assert(err, IsNil)
	uc := NewUserConfig()
	c.Assert(uc.LoadConfig(filePath), Equals, true)
	
	cluster.PutStoreWithLabels(1, "zone", "z1", "rack", "r1", "host", "h1")
	cluster.PutStoreWithLabels(2, "zone", "z2", "rack", "r2", "host", "h2")
	cluster.PutStoreWithLabels(3, "zone", "z3", "rack", "r3", "host", "h3")
	cluster.PutStoreWithLabels(4, "zone", "z4", "rack", "r4", "host", "h4")
	cluster.PutStoreWithLabels(5, "zone", "z5", "rack", "r5", "host", "h5")
	schedulers := ProduceScheduler(uc, opc, cluster)
	c.Assert(schedulers, NotNil)
	c.Assert(len(schedulers), Equals, 4)
}

func (s *testUserConfigSuite) TestGetStoreId(c *C) {
	opt := mockoption.NewScheduleOptions()
	cluster := mockcluster.NewCluster(opt)
	cluster.PutStoreWithLabels(1, "zone", "z1", "rack", "r1", "host", "h1")
	cluster.PutStoreWithLabels(2, "zone", "z2", "rack", "r2", "host", "h2")
	cluster.PutStoreWithLabels(3, "zone", "z3", "rack", "r3", "host", "h3")
	cluster.PutStoreWithLabels(4, "zone", "z4", "rack", "r4", "host", "h4")
	cluster.PutStoreWithLabels(5, "zone", "z5", "rack", "r5", "host", "h5")

	filePath, err := filepath.Abs("../conf/test_config.toml")
	c.Assert(err, IsNil)
	uc := NewUserConfig()
	c.Assert(uc.LoadConfig(filePath), Equals, true)
	schedule.PluginsMapLock.RLock()
	defer schedule.PluginsMapLock.RUnlock()
	for name, stores := range uc.GetStoreId(cluster) {
		for i, store := range stores {
			for _, label := range schedule.PluginsMap[name].Stores[i].StoreLabel {
				c.Assert(strings.Contains(label.Value, strconv.Itoa(int(store))), Equals, true)
			}
		}
	}
}

func (s *testUserConfigSuite) TestGetInterval(c *C) {
	filePath, err := filepath.Abs("../conf/test_config.toml")
	c.Assert(err, IsNil)
	uc := NewUserConfig()
	c.Assert(uc.LoadConfig(filePath), Equals, true)
	schedule.PluginsMapLock.RLock()
	defer schedule.PluginsMapLock.RUnlock()
	for name, interval := range uc.GetInterval() {
		c.Assert(interval.Begin, Equals, schedule.PluginsMap[name].GetInterval().GetBegin())
		c.Assert(interval.End, Equals, schedule.PluginsMap[name].GetInterval().GetEnd())
	}
}

// test irreconcilable conflict
func (s *testUserConfigSuite) TestConflict(c *C) {
	c.Assert(len(IfOverlap([]uint64{1, 2, 3}, []uint64{2, 3, 4})), Equals, 2)
	c.Assert(len(IfOverlap([]uint64{1, 2, 3}, []uint64{4, 5, 6})), Equals, 0)

	opt := mockoption.NewScheduleOptions()
	cluster := mockcluster.NewCluster(opt)
	htStream := mockhbstream.NewHeartbeatStream()
	opc := schedule.NewOperatorController(cluster, htStream)
	filePath, err := filepath.Abs("../conf/test_conflict.toml")
	c.Assert(err, IsNil)
	uc := NewUserConfig()
	c.Assert(uc.LoadConfig(filePath), Equals, false)
	c.Assert(uc.IfConflict(), Equals, true)
	schedulers := ProduceScheduler(uc, opc, cluster)
	c.Assert(schedulers, NotNil)
	c.Assert(len(schedulers), Equals, 0)
}
