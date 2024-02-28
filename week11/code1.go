/*
为了支持抢占式任务调度中的续约失败情况，我们需要在查询条件中加入对任务续约时间的检查。具体来说，我们需要考虑以下两种情况：

1. 任务还没有被调度过，即满足原有的查询条件。
2. 任务曾经被调度过，但是后续的续约操作失败了。这可以通过检查任务的最后续约时间（例如`last_renew_time`字段）是否小于当前时间减去允许的最大续约间隔（`max_renew_interval`）来判断。
因此，我们需要修改查询条件以包含第二种情况。下面是修改后的代码示例：
*/

package main

import (
	"time"

	"gorm.io/gorm"
)

type Job struct {
	// 假设Job结构体中包含以下字段
	ID             uint      // 任务ID
	Status         string    // 任务状态
	NextTime       time.Time // 下次调度时间
	LastRenewTime  time.Time // 最后一次续约时间
}

func fetchJob(db *gorm.DB) (Job, error) {
	now := time.Now()
	var j Job

	// 增加了对续约失败情况的查询条件
	err := db.Where("(next_time <= ? AND status = ?) OR (last_renew_time <= ? AND status = ?)", now, "waiting", now.Add(-maxRenewInterval), "running").First(&j).Error
	if err != nil {
		return Job{}, err
	}

	return j, nil
}

var (
	// 假设最大续约间隔为30秒
	maxRenewInterval = 30 * time.Second
)
