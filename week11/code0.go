/*
### 解决方案思路

在不引入额外中间件的前提下，我们可以通过Redis自身的功能来实现一个基于节点负载的分布式锁选择机制。主要思路是利用Redis的有序集合(sorted set)，将节点的负载作为分数(score)，节点标识作为成员(member)存储在有序集合中。这样，我们就可以轻松地根据负载（即分数）选择负载最低的节点。

#### 步骤概述

1. **节点负载上报**：每个节点定时将自己的负载情况（随机生成的0-100数字）上报到Redis的有序集合中，使用节点标识作为成员，负载值作为分数。

2. **选择节点**：当需要选择节点进行热榜计算时，从有序集合中选择分数（负载）最低的节点。

3. **分布式锁**：选中节点后，尝试获取分布式锁以确保同一时间只有一个节点进行热榜计算。如果选中的节点因为某些原因（如负载突然增高）需要放弃执行权，它可以释放分布式锁，让其他节点有机会获取锁并执行计算。

4. **负载变化和锁的续期**：节点在执行热榜计算期间，应定时更新自己的负载信息到Redis，并检查自己是否仍持有分布式锁（或续期锁）。
*/

//**节点负载上报**：

func reportLoad(redisClient *redis.Client, nodeID string, load int) {
    redisClient.ZAdd("node_loads", redis.Z{Score: float64(load), Member: nodeID})
}

//**选择节点**：

func selectNode(redisClient *redis.Client) string {
    // 获取负载最低的节点
    result, err := redisClient.ZRangeWithScores("node_loads", 0, 0).Result()
    if err != nil || len(result) == 0 {
        return ""
    }
    return result[0].Member.(string)
}

// **获取和释放分布式锁**：

func tryLock(redisClient *redis.Client, nodeID string) bool {
    // 尝试获取分布式锁
    result, err := redisClient.SetNX("hotlist_lock", nodeID, lockTimeout).Result()
    return err == nil && result
}

func releaseLock(redisClient *redis.Client, nodeID string) {
    // 释放分布式锁
    redisClient.Del("hotlist_lock")
}
/*
#### 极端情况分析

- **选中最差的节点**：由于我们的方案允许一定的延迟，在极端情况下，可能在选择节点后节点的负载突然增高，但这种情况应该较为罕见。通过定时更新负载信息和检查分布式锁的持有情况，可以一定程度上减少这种情况的发生。

- **节点宕机**：如果选中的节点在获取锁后宕机，由于我们设置了锁的超时时间，锁会在超时后自动释放，其他节点可以重新竞争锁。为了减少宕机对热榜计算的影响，可以设置较短的锁超时时间，并在节点上实现健康检查机制，及时从有序集合中移除不健康的节点。
*/
