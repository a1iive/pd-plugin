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