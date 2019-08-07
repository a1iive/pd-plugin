<!--
This is a template for TiDB's change proposal process, documented [here](./README.md).
-->

# Proposal: 可插入式的PD调度器<!-- Title -->

- Author(s): 梁晋荣[https://github.com/dimstars](https://github.com/dimstars)，胡皓胜[https://github.com/a1iive](https://github.com/a1iive)  <!-- Author Name, Co-Author Name, with the link(s) of the GitHub profile page -->
- Last updated: 2019-08-02  <!-- Date -->
- Discussion at: todo  <!-- https://github.com/pingcap/tidb/issues/XXX -->

## Abstract

- 目前PD的调度策略是平衡负载，但有时候用户有更为细致的需求，比如某些数据在某段时间会被大量访问，需要将其部署到高性能的结点上，就要求PD让用户能够自定义调度规则，管理数据。
- 新增调度插件，解析规则，生成scheduler调度用户指定的数据。在PD中添加解决调度冲突的代码，使原有调度不会影响用户的调度。
- 可插入式的PD调度器，支持用户自定义调度规则，支持动态更新配置文件，在满足用户需求的前提下尽可能平衡集群负载。可能出现集群负载不均衡，增加的检查机制也会带来少量额外的开销。
<!--
A short summary of the proposal:
- What is the issue that the proposal aims to solve?
- What needs to be done in this proposal?
- What is the impact of this proposal?
-->

## Background

- 某些应用场景下，TiKV的用户可能发出短时间高爆发的访问，比如上下班时间对共享单车的需求。而PD调度采用的是动态平衡的策略，虽然能够根据访问量动态调整数据分布，但被动的调度还是有些力不从心。也就是说，目前的PD缺乏满足特定用户需求的能力，不能根据不同的应用场景改变调度的策略。
- 本提议期望PD能够支持用户自定义调度规则，动态获取用户的配置并应用到调度中，解决用户规则和原有调度之间的冲突，在满足用户要求的前提下尽可能平衡负载。
<!--
An introduction of the necessary background and the problem being solved by the proposed change:
- The drawback of the current feature and the corresponding use case
- The expected outcome of this proposal.
-->

<!--
## Proposal
### new named concepts
- LeaderDistribution
- RegionDistribution
-->

### overview
通过动态地解析用户配置文件，获取和应用用户自定义规则，满足用户特定的调度需求。
### How it works?
- 利用toml配置文件解析用户自定义规则，解析操作在插件中完成，然后根据用户定义的规则生成对应的scheduler，完成相应调度。
- 用户可以随时修改配置文件，之后向PD进程发起SIGUSR1系统调用信号就可以使其重新读取配置。
- 我们将规则转化为具体资源（region、leader、store）的调度操作，存在PD中，在原有调度选择source或target时，过滤掉用户指定的部分数据，在其他数据中选择进行调度。
### What needs to be changed to implement this design?
- 编写golang plugin，能够解析toml配置文件并生成对应的scheduler，当新的规则出现时，转化为PD可识别的格式存储到PD中。
- 在PD启动时注册插件，同时调用插件获取用户自定义规则，存到内存中。
- 添加处理调度冲突的代码
### What may be positively influenced by the proposed change?
支持用户自定义调度规则，支持动态更新配置文件，更好地适应各种应用场景。
### What may be negatively impacted by the proposed change?
可能出现集群负载不均衡，增加的检查机制也会带来少量额外的开销。
<!--
A precise statement of the proposed change:
- The new named concepts and a set of metrics to be collected in this proposal (if applicable)
- The overview of the design.
- How it works?
- What needs to be changed to implement this design?
- What may be positively influenced by the proposed change?
- What may be negatively impacted by the proposed change?
-->

## Rationale
### How other systems solve the same issue?
- 提供抽象化的调度规则供用户定义。
- 使用配置文件获取用户自定义规则。
- 将抽象规则解析为具体的调度。
- 处理原生规则和用户规则的冲突。
### What other designs have been considered and what are their disadvantages?
为了避免冲突，只使用原有调度或者用户自定义调度，设置开关进行控制。
但这样做需要用户定义的规则具有较高的可行性，对自定义规则的抽象和用户的专业水平要求较高，用的不好很有可能导致数据不一致或者集群崩溃。
### What is the advantage of this design compared with other designs?
- 修改插件的代码不影响PD的业务逻辑。
- 用户自定义的调度规则可以很灵活，不管如何修改规则，插件只需要解析成对具体资源的调度就可以存到PD中。
- 没有改变PD原有的框架结构，比较稳定。
- 基本可以解决两套调度策略间的冲突，在满足用户要求的前提下尽可能平衡负载。
### What is the disadvantage of this design?
- 由于用户定义的调度只考虑到部分数据，对整个集群没有宏观调控，可能出现集群负载不均衡。
- 增加的检查机制也会带来少量额外的开销。
### What is the impact of not doing this?
缺乏满足特定用户需求的能力，不能根据不同的应用场景改变调度的策略。
<!--
A discussion of alternate approaches and the trade-offs, advantages, and disadvantages of the specified approach:
- How other systems solve the same issue?
- What other designs have been considered and what are their disadvantages?
- What is the advantage of this design compared with other designs?
- What is the disadvantage of this design?
- What is the impact of not doing this?
-->

## Compatibility
### Does this proposal make TiDB not compatible with the old versions?
No.
### Does this proposal make TiDB more compatible with MySQL?
No.
<!--
A discussion of the change with regard to the compatibility issues:
- Does this proposal make TiDB not compatible with the old versions?
- Does this proposal make TiDB more compatible with MySQL?
-->

## Implementation
### User-defined rule
#### 1. 通过label指定一段key range对应数据的leader到特定store上。
- 如果目标store不存在指定region的peer，则将leader peer移动到该store；
- 如果存在peer但他不是leader，则转移该region的leader；
- 如果指定region的leader就在目标store上，不做操作。
#### 2. 通过label指定一段key range对应数据的region副本到某些store上，或者指明这部分数据不存在某些store上。
- 如果目标store数量大于或等于region的副本数，则将其他store上的该region的peer移动到没有该region副本的目标store上；
- 如果目标store数量小于region的副本数，则随机选取region的部分副本移动到目标store上，直到目标store上都具有该region的一个副本，剩下的副本出于稳定性考虑不做处理，可以提示用户重新配置，或者在解析时就拦截用户的不合理配置。
#### 3. 指定时间的调度
- 在上述两种规则的基础上指定时间，start_time指定开始调度的时间，在结束时间end_time后解除对指定数据的限制，以便其他规则对其进行调度。
### Profile parsing
利用toml配置文件解析用户自定义规则，解析操作在插件中完成，然后根据用户定义的规则生成对应的scheduler。用户可以随时修改配置文件，之后向PD进程发起SIGUSR1系统调用信号就可以使其重新读取配置。
### Conflict handling
- 原生规则移动用户指定的数据

	举例：用户自定义规则要求将数据a-d移动到指定store1上，原生规则（如balance_leader）出于平衡考虑，需要在store1上选取leader转移到store2上，就很可能把a-d中的某些数据移走，这样用户的规则就被破坏了。

	解决：把用户的规则存到PD中，balance_leader在选择source region时，检查用户自定义规则，过滤掉a-d的数据对应region，就不会出现上述情况。
- 用户自定义规则间的冲突

	举例：用户定义的key range存在交叉，或由此得到regionId存在交叉，比如一条规则指定了region1-6的leader到store2上，又指定了region5-10的leader到store3上，那么如何决定region5、6的去向就成了问题。

	解决：对于这种情况，采取先来后到的方式，后一条规则不能拥有前面已指定数据的控制权。
- 用户规则移动大量数据造成不平衡

	举例：在PD的良好调度下，现在所有的5个store负载都维持在60%左右，这时用户希望store1上调度很多region过去，可能占一个store的40%大小，这时store1短时间内就会接近满负载，而后续需要一段时间才能逐步平衡，期间集群的负载很不均衡，可用性和稳定性都可能下降。

	解决：在执行自定义规则前需要对其进行评估，如果这些操作执行后会对集群有较大的影响，则应该考虑是否执行或者是否立即执行。可以的话，将大量数据的移动变得更为平滑，一边移入指定数据，一边移出其他无关数据，在动态平衡中完成规则的调度。如果用户的规则会使集群陷入危险脆弱的状态，则暂时不执行并警告用户。

<!--
A detailed description for each step in the implementation:
- Does any former steps block this step?
- Who will do it?
- When to do it?
- How long it takes to accomplish it?
-->

<!--
## Open issues (if applicable)
A discussion of issues relating to this proposal for which the author does not know the solution. This section may be omitted if there are none.
-->