# Proposal: Pluggable PD scheduler <!-- Title -->

- Author(s): Liang Jinrong[https://github.com/dimstars](https://github.com/dimstars)，Hu Haosheng[https://github.com/a1iive](https://github.com/a1iive)
- Last updated: 2019-08-02
- Discussion at: https://github.com/pingcap/pd/issues
## Abstract

- Currently, PD's scheduling strategy is load-balancing, but sometimes users have more detailed requirements. For example, some data will be frequently accessed in a certain period of time, and it needs to be deployed to high-performance nodes. PD needs to allow users to customize scheduling rules and manage data.
- Add scheduling plugins to parse rules and generate scheduler to schedule user-specified data. Add code to PD to resolve scheduling conflicts so that the original scheduling will not affect the user's scheduling.
- The pluggable PD scheduler supports user-defined scheduling rules, supports dynamic update of configuration files, and balances the cluster load as much as possible while satisfying user requirements. Cluster load imbalance may occur, and the added inspection mechanism will bring a small amount of additional overhead.

## Background

- In some application scenarios, TiKV users may issue short bursts of access, such as the demand for Shared bikes during rush hours. PD scheduling adopts a dynamic balance strategy. Although it can dynamically adjust the data distribution according to the visits, it cannot be deployed in advance, so passive scheduling seems to be a little difficult.
- In addition, load balancing assumes that all servers perform similarly.While server performance can vary significantly, users want critical data on high-performance machines.That is to say, PD currently lacks the ability to meet specific user needs and cannot change scheduling strategies according to different application scenarios.
- This proposal expects PD to support user-defined scheduling rules, dynamically acquire user configurations and apply them to scheduling, resolve conflicts between user rules and original scheduling, and balance load as much as possible on the premise of meeting user requirements.

### overview
By dynamically parsing the user configuration file, obtain and apply user-defined rules to meet the specific scheduling needs of users.
### How it works?
- The "toml" configuration file is used to parse the user-defined rules, and the parsing operation is completed in the plugin. Then, the corresponding scheduler is generated according to the user-defined rules to complete the corresponding scheduling.
- The user can modify the configuration file at any time and then issue a SIGUSR1 system call signal to the PD process to re-read the configuration.
- We translate rules into scheduling operations of specific resources (region, leader, store), which are stored in PD. When the original scheduling selects source or target, part of the data specified by the user is filtered out, and other data is selected for scheduling.
### What needs to be changed to implement this design?
- Write the golang plugin to parse the toml configuration file and generate the appropriate scheduler.When a new rule appears, it is converted to a format recognized by PD and stored in PD.
- Register the plugin at PD startup and call the plugin to retrieve user-defined rules and store them in memory.
- Add code to handle scheduling conflicts.
### What may be positively influenced by the proposed change?
The proposal supports user-defined scheduling rules and dynamic update of configuration files to better adapt to various application scenarios.
### What may be negatively impacted by the proposed change?
The cluster load may be unbalanced, and the additional checking mechanism may bring a small additional overhead.

## Rationale
### How other systems solve the same issue?
- Provide abstract scheduling rules for user definition.
- Use configuration files to get user-defined rules.
- Resolve abstract rules into concrete schedules.
- Handle conflicts between native rules and user rules.
### What other designs have been considered and what are their disadvantages?
To avoid conflicts, only use the original schedule or user-defined schedule, and set the switch for control.
But doing so requires user-defined rules to be highly feasible.High level of abstraction of custom rules and professional level of users are required.Poor use can lead to inconsistent data or cluster crashes.
### What is the advantage of this design compared with other designs?
- Modifying the plugin code does not affect PD's business logic.
- User-defined scheduling rules can be flexible. Regardless of how the rule is modified, the plugin simply needs to parse it into a schedule for a specific resource and store it in PD.
- Modifying the plugin code does not affect PD's business logic.User-defined scheduling rules can be flexible.Regardless of how the rule is modified, the plugin simply needs to parse it into a schedule for a specific resource and store it in PD.It does not change the original frame structure of PD.It's pretty stable.
- The conflicts between the two scheduling policies can be basically solved and the load can be balanced as much as possible under the premise of meeting the requirements of users.
### What is the disadvantage of this design?
- Since the user-defined scheduling only considers part of the data, there is no macro-control for the entire cluster, and the cluster load may be unbalanced.
- The added checking mechanism also brings a small additional overhead.
### What is the impact of not doing this?
Lack of ability to meet the needs of specific users, can not change the scheduling policy according to different application scenarios.

## Compatibility
### Does this proposal make TiDB not compatible with the old versions?
No.
### Does this proposal make TiDB more compatible with MySQL?
No.

## Implementation
### User-defined rule
#### 1. Specify a leader of data corresponding to a key range on a specific store through label.
- If the target store does not have peer in the specified region, the leader peer is moved to the store.
- If there is a peer but it is not a leader, then transfer the leader of the region.
- If the leader of the region is on the target store, do nothing.
- Given multiple stores, leaders are distributed evenly across those stores.
#### 2. Specify a region copy of the corresponding data of a key range on some stores by label.
- If the number of target stores is greater than or equal to the number of copies of the region, move peers of the region from other stores to the target store which has no copy of the region.
- If the number of target stores is less than the number of copies of the region, randomly select some copies of the region and move them to the target store until all target stores have one copy of the region, leaving the remaining copies unprocessed.
#### 3. Scheduling of specified times
- Specify the time range on the above two rules, start_time specifies the time to start scheduling, and the restriction on specified data is lifted after end_time, so that other rules can schedule it.
### Profile parsing
The user-defined rules are parsed using the toml configuration file, the parsing is done in the plugin, and the corresponding scheduler is generated according to the user-defined rules.The user can modify the configuration file at any time and then issue a SIGUSR1 system call signal to the PD process to reread the configuration.
### Conflict handling
#### 1. Native rules move user-specified data

- Strategy: No other scheduler is allowed to move user-specified data.

	Example: User-defined rules require that the data a-d be moved to the specified store1, and native rules (such as balance_leader) that require the selected leader on store1 to be moved to store2 for the sake of balance are likely to move some data from a-d, thus breaking the user's rules.
	
	Solution: Store the user's rules in PD. When balance_leader selects the source region, he checks the user-defined rules and filters out the corresponding region of a-d data, and the above situation will not occur.

#### 2. Conflicts between user-defined rules

- Strategy 1: Overlapping key ranges and time periods between rules of the same type, reporting errors and stopping execution.

	Example: There are two rules about the distribution of leaders. A rule assigned leaders of  region1-6 to store2 and leaders of  region5-10 to store3, creating a problem deciding where to put the leaders of  regions 5 and 6.

	Resolution: In this case, you can only refuse to execute the user's rules and prompt the user to modify the configuration file.
- Strategy 2: Key ranges overlap between different types of rules and time periods overlap. If the target store has conflicts, it will not be executed.If the target store can be satisfied at the same time, proceed.

	Example: One rule assigned leaders of region1-6 to store2, and another named all copies of region5-10 to store3, 4, and 5, creating a problem deciding where to put region5 and 6.

	Solution: The store specified by the rule defining the leader is not included in the store specified by the rule defining the region, so there is a conflict and it cannot be executed.If the previous rule specifies one of store3, 4, and 5, then both rules can be satisfied and continue.Or if the latter rule only specifies two stores, then one of the peers can be on any other store, and the two rules can be satisfied simultaneously.
#### 3. User rules move large amounts of data causing imbalances

- Strategy: Check whether the target store can load more data at the time of scheduling. If not, suspend the scheduling.

	Example: Under the good scheduling of PD, all 5 store loads are now maintained at around 50%, at this time, users want to schedule many regions to store1, which may account for 60% of the size of a store, at this time, store1 will be close to full load in a short time, and the availability and stability may decline.

	Solution: Custom rules need to be evaluated before they are executed, and if they have a significant impact on the cluster after they are executed, you should consider whether or not to execute them immediately.If possible, smooth the movement of large amounts of data.Moves in specified data while moving out other irrelevant data.Complete the scheduling of rules in dynamic balance.

## Test
### 测试方案
#### 1. 无冲突的综合功能测试

简述：多move-leader多move-region，兼顾单个store和多个store  
冲突：无  
详细说明：定义一系列无冲突的调度规则，即涉及两种类型，且key range互不重叠的多个规则，既有单个目标store也有多个目标store   
预期结果：顺利运行，region最终分布与配置文件描述一致

#### 2. 不可调和的冲突测试

- 同种规则间的冲突测试  

	简述：同种规则间key range重叠，时间有交叉或不交叉  
	冲突：有  
	详细说明：一种类型的两条规则间，key range存在重叠，时间段也有交叉。另一种类型的两条规则间，key range存在重叠，但时间段没有交叉  
	预期结果：时间存在交叉的两条规则会报错，时间不交叉的两条规则不会报错

- 不同类型规则间的冲突测试

	简述：不同类型规则间key range重叠，时间交叉，目标store无交集  
	冲突：有  
	详细说明：定义两组不同类型的规则，一组不同类型的规则间key range存在重叠，时间段也有交叉，并且“leader”类型规则的目标store与“region”类型规则的目标store不相交。另一组不同类型的规则间key range存在重叠，时间段也有交叉，但是“leader”类型规则的目标store与“region”类型规则的目标store存在交集  
	预期结果：目标store无交叉关系的规则会报错，目标store有交叉关系的规则不会报错，但最终不能执行

#### 3. 可调和的冲突测试

简述：不同类型规则间key range重叠，时间交叉，目标store存在交集  
冲突：有  
详细说明：定义不同类型的规则，规则间key range存在重叠，时间段也有交叉，但是“leader”类型规则的目标store与“region”类型规则的目标store存在交集  
预期结果：顺利解析执行，最终结果与满足配置要求，重叠的key对应的region既在“region”规则指定的store上，它的leader也在“leader”规则指定的store上

### 集群配置
#### 1 服务器

|      系统       | CPU  | 内存 | 硬盘 |
| :-------------: | :--: | :--: | :--: |
| CentOS 7.6 64位 |  16  | 32G  | 500G |


#### 1 PD

```bash
./bin/pd-server --name=pd1 \
                --data-dir=pd1 \
                --client-urls="http://127.0.0.1:2379" \
                --peer-urls="http://127.0.0.1:2380" \
                --initial-cluster="pd1=http://127.0.0.1:2380" \
                --log-file=pd1.log
```

#### 5 TiKV

运行脚本[runtikv](https://github.com/a1iive/pd-plugin/blob/master/runtikv.sh)(使用时修改文件内“tikvdir”为tikv所在目录)

```bash
./runtikv.sh 5
```

### 无冲突的综合功能测试

#### 测试方法

- 更换配置[user_config](https://github.com/a1iive/pd-plugin/blob/master/conf/user_config.toml)内容为[test1_config](https://github.com/a1iive/pd-plugin/blob/master/plugin_test/test1_config.toml)

- 应用用户配置

```bash
./plugin/signal.sh
```

#### 用户自定义规则

| 类型   |    名称 | StartKey | EndKey   | 目标StoreLabel                                               |
| ------ | ------: | -------- | -------- | ------------------------------------------------------------ |
| leader | Leader0 | ...1773… | ...3943… | “z3”,   “r3”, “h3”                                           |
| leader | Leader1 | …3943... | …5743…   | “z1”,   “r1”, “h1”                                                           “z5”,   “r5”, “h5” |
| region | Region0 | …5743…   | …6973…   | “z1”,   “r1”, “h1”                                                     “z2”,   “r2”, “h2”                                                    “z3”,   “r3”, “h3” |
| region | Region1 | …6973…   | …9973…   | “z4”,   “r4”, “h4”                                                         “z5”, “r5”,   “h5” |

定义了两种类型的4个规则，各个规则间的key range不重叠，调度leader的规则既有指定单个store也有指定多个store的，调度region的规则既有指定等同region副本数量的store也有少于副本数量的store

#### 解析结果

| 类型   | 名称     | RegionId               | 目标StoreLabel                                               | StoreId   |
| ------ | -------- | ---------------------- | ------------------------------------------------------------ | --------- |
| leader | Leader-0 | 5855，5836，5841，5846 | “z3”,   “r3”, “h3”                                           | 2         |
|        | Leader-1 | 5825，5830             | “z1”,   “r1”, “h1”   “z5”,   “r5”, “h5”                      | 10，3     |
| region | Region-0 | 5818，5789，5814       | “z1”,   “r1”, “h1”   “z2”,   “r2”, “h2”   “z3”,   “r3”, “h3” | 10，13，2 |
| 1      | Region-1 | 5795，5808，5801，4    | “z4”,   “r4”, “h4”   “z5”,   “r5”, “h5”                      | 1，3      |

当前获取的RegionId和StoreId，与上表对应

![move leader](https://github.com/a1iive/pd-plugin/blob/master/plugin_test/test1-1.png)

![move region](https://github.com/a1iive/pd-plugin/blob/master/plugin_test/test1-2.png)

#### 测试结果

|  类型  |   名称   | RegionId | StoreId（按序） |      |      |      |      |
| :----: | :------: | :------: | :-------------: | :--: | :--: | :--: | :--: |
|        |          |          |       10        |  13  |  2   |  1   |  3   |
| leader | Leader-0 |   5855   |        F        |      |  L   |      |  F   |
|        |          |   5836   |        F        |      |  L   |      |  F   |
|        |          |   5841   |        F        |  F   |  L   |      |      |
|        |          |   5846   |        F        |      |  L   |      |  F   |
|        | Leader-1 |   5825   |                 |  F   |  F   |      |  L   |
|        |          |   5830   |        F        |      |  F   |      |  L   |
| region | Region-0 |   5818   |        F        |  L   |  F   |      |      |
|        |          |   5789   |        L        |  F   |  F   |      |      |
|        |          |   5814   |        F        |  L   |  F   |      |      |
|        | Region-1 |   5795   |        F        |      |      |  L   |  F   |
|        |          |   5808   |                 |  F   |      |  F   |  L   |
|        |          |   5801   |                 |  L   |      |  F   |  F   |
|        |          |    4     |                 |  F   |      |  L   |  F   |
|   -    |   其他   |   5851   |        L        |      |      |  F   |  F   |

其中“leader”类型的规则只需要指定region的leader在用户指定的某一个store上即可；“region”类型的规则需要指定region的所有副本都分布到指定store上（如果store数少于region的副本数，那么只需填满指定region即可，多出来的peer可任意分布）

从结果来看，规则Leader-0覆盖region的leader都在指定的store（id=2）上，规则Leader-1覆盖region的leader都在指定的store（id=3）上。规则Region-0覆盖region的peer都在指定的store（id=10、13、2）上，至于它们的leader在哪一个store上则不是规则指定的内容。规则Region-1覆盖region的peer不全在指定的store（id=1、3）上，是因为store数量小于副本数，所以只需满足指定的store都有对应region的副本即可  

测试结果与预期相符，插件能在无冲突的情况下完成用户自定义调度

Region分布结果见[test1_result](https://github.com/a1iive/pd-plugin/blob/master/plugin_test/test1_result.txt)

### 同种规则间的冲突测试

#### 测试方法

- 更换配置[user_config](https://github.com/a1iive/pd-plugin/blob/master/conf/user_config.toml)内容为[test2_config](https://github.com/a1iive/pd-plugin/blob/master/plugin_test/test2_config.toml)

- 应用用户配置

```bash
./plugin/signal.sh
```

#### 用户自定义规则

| 类型   | 名称     | StartKey | EndKey | 时间范围                                          | StoreLabel                                                   |
| ------ | -------- | -------- | ------ | ------------------------------------------------- | ------------------------------------------------------------ |
| leader | Leader-0 | …1773…   | …9973… | 2019-**08**-05-14:55:00 ~ 2019-**08**-30-10:30:00 | “z3”,   “r3”, “h3”                                           |
|        | Leader-1 | …1773…   | …9973… | 2019-**07**-05-14:55:00 ~ 2019-**07**-30-10:30:00 | “z1”,   “r1”, “h1”   “z5”,   “r5”, “h5”                      |
| region | Region-0 | …1773…   | …9973… | 2019-08-05-14:55:00   ~ 2019-08-30-10:30:00       | “z1”, “r1”,   “h1”   “z2”,   “r2”, “h2”   “z3”,   “r3”, “h3” |
|        | Region-1 | …1773…   | …9973… | 2019-08-05-14:55:00   ~ 2019-08-30-10:30:00       | “z4”,   “r4”, “h4”   “z5”,   “r5”, “h5”                      |

定义了两种类型的4个规则，所有规则间的key range都有重叠，“region”类型的规则间还存在时间范围的重叠，而“leader”类型的两条规则时间范围不重叠

#### 测试结果

![解析结果](https://github.com/a1iive/pd-plugin/blob/master/plugin_test/test2-1.png)

规则Region-0和Region-1之间存在冲突，而规则Leader-0和Leader-1之间没有冲突，因为这两条规则虽然key range重叠但是时间没有交叉，所以可以在各自的时间段进行调度而不会互相影响。  

对于同种规则间的冲突，默认为是用户配置了错误的定义，应该报告给用户修改配置文件，避免冲突

### 不同类型规则间的冲突测试
#### 测试方法

- 更换配置[user_config](https://github.com/a1iive/pd-plugin/blob/master/conf/user_config.toml)内容为[test3_config](https://github.com/a1iive/pd-plugin/blob/master/plugin_test/test3_config.toml)

- 应用用户配置

```bash
./plugin/signal.sh
```

#### 用户自定义规则

| 类型   | 名称     | StartKey | EndKey | 目标StoreLabel                                               |
| ------ | -------- | -------- | ------ | ------------------------------------------------------------ |
| leader | Leader-0 | …5743…   | …9973… | “z2”,   “r2”, “h2”                                           |
|        | Leader-1 | …3943…   | …5743… | “z1”,   “r1”, “h1”   “z5”,   “r5”, “h5”                      |
| region | Region-0 | …5743…   | …6973… | “z1”,   “r1”, “h1”   “z2”,   “r2”, “h2”   “z3”,   “r3”, “h3” |
|        | Region-1 | …6973…   | …9973… | “z3”,   “r3”, “h3”   “z4”,   “r4”, “h4”   “z5”,   “r5”, “h5” |

构造了“leader”和“region”两种类型规则间的冲突，即Leader-0和Region-0、Region-1都有key range的重叠

#### 测试结果

![解析结果](https://github.com/a1iive/pd-plugin/blob/master/plugin_test/test3-1.png)

规则Leader-0和Region-0、Region-1都有key range的重叠，需要进一步检查。Leader-0指定的store与Region-0指定的几个store相交，所以这两条规则是可以同时满足的，不存在冲突，而Region-1指定的store与Leader-0指定的store不相交，所以它们无法同时满足，因而产生冲突

### 可调和的冲突测试
#### 测试方法

- 更换配置[user_config](https://github.com/a1iive/pd-plugin/blob/master/conf/user_config.toml)内容为[test4_config](https://github.com/a1iive/pd-plugin/blob/master/plugin_test/test4_config.toml)

- 应用用户配置

```bash
./plugin/signal.sh
```

#### 用户自定义规则

| 类型   | 名称     | StartKey | EndKey | 目标StoreLabel                                               |
| ------ | -------- | -------- | ------ | ------------------------------------------------------------ |
| leader | Leader-0 | …1773…   | …3943… | “z3”,   “r3”, “h3”                                           |
|        | Leader-1 | …5743…   | …9973… | “z1”,   “r1”, “h1”   “z5”,   “r5”, “h5”                      |
| region | Region-0 | …5743…   | …6973… | “z1”,   “r1”, “h1”   “z2”,   “r2”, “h2”   “z3”,   “r3”, “h3” |
|        | Region-1 | …6973…   | …9973… | “z3”,   “r3”, “h3”    “z4”,   “r4”, “h4”   “z5”,   “r5”, “h5” |

构造了“leader”和“region”两种类型规则间的冲突，即Leader-1和Region-0、Region-1都有key range的重叠，但是Leader-1指定了两个store，并且分别包含在两条“region”规则指定的store中

#### 规则解析

| 类型   | 名称     | RegionId                        | 目标StoreLabel                                               | StoreId |
| ------ | -------- | ------------------------------- | ------------------------------------------------------------ | ------- |
| leader | Leader-0 | 7362，7350，7356，7343          | “z3”,   “r3”, “h3”                                           | 8       |
|        | Leader-1 | 6792，6819，6798，6811，6804，3 | “z1”,   “r1”, “h1”   “z5”,   “r5”, “h5”                      | 7，1    |
| region | Region-0 | 6792，6819                      | “z1”,   “r1”, “h1”   “z2”,   “r2”, “h2”   “z3”, “r3”,   “h3” | 7，2，8 |
|        | Region-1 | 6798，6811，6804，3             | “z3”,   “r3”, “h3”    “z4”,   “r4”, “h4”   “z5”,   “r5”, “h5” | 8,11，1 |

当前获取的RegionId和StoreId，与上表对应  

![move leader](https://github.com/a1iive/pd-plugin/blob/master/plugin_test/test4-1.png)

![move region](https://github.com/a1iive/pd-plugin/blob/master/plugin_test/test4-2.png)

规则Leader-1和Region-0、Region-1都有key range的重叠，所以需要进一步检查。

![result](https://github.com/a1iive/pd-plugin/blob/master/plugin_test/test4-3.png)

由于规则Leader-1指定的目标store和Region-0、Region-1的都有交叉，所以它们都可以同时满足，冲突可以避免，可见对应的scheduler创建成功

#### 测试结果

|  类型  |   名称   |       RegionId       | StoreId（按序） |      |      |      |      |
| :----: | :------: | :------------------: | :-------------: | :--: | :--: | :--: | :--: |
|        |          |                      |        7        |  2   |  8   |  11  |  1   |
| leader | Leader-0 |         7362         |        F        |      |  L   |  F   |      |
|        |          |         7350         |                 |  F   |  L   |  F   |      |
|        |          |         7356         |        F        |      |  L   |  F   |      |
|        |          |         7343         |        F        |  F   |  L   |      |      |
|        | Leader-1 | 见Region-0、Region-1 |                 |      |      |      |      |
| region | Region-0 |         6792         |        L        |  F   |  F   |      |      |
|        |          |         6819         |        L        |  F   |  F   |      |      |
|        | Region-1 |         6798         |                 |      |  F   |  F   |  L   |
|        |          |         6811         |                 |      |  F   |  F   |  L   |
|        |          |         6804         |                 |      |  F   |  F   |  L   |
|        |          |          3           |                 |      |  F   |  F   |  L   |
|   -    |   其他   |         7274         |        F        |  L   |      |      |  F   |
|        |          |         7368         |        L        |  F   |      |      |  F   |
|        |          |         6824         |        F        |  F   |      |  L   |      |

规则Leader-0覆盖region的leader都在指定store（id=8）上。规则Region-0覆盖的region所有副本都在指定store（id=7、2、8）上，规则Region-1覆盖的region所有副本都在指定store（id=8、11、1）上

规则Leader-1覆盖的region正好是Region-0和Region-1的所有region之和，这条规则要求对应region的leader必须在store-7或store-1上，表中可以看出结果刚好满足这一条件。  
也就是说，只要“leader”规则与“region”规则指定的store存在交叉关系，那么即便region有重叠也是可以接受的。

Region分布结果见[test4_result](https://github.com/a1iive/pd-plugin/blob/master/plugin_test/test4_result.txt)