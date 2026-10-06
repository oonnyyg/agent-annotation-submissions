package main

import "fmt"

// Job 状态常量。
const (
	StatusPending  = "pending"  // 待打印（可能被暂挂）
	StatusPrinting = "printing" // 已开始，等待逐张确认
	StatusDone     = "done"     // 全部确认完成
	StatusCanceled = "canceled" // 已取消
)

// Job 表示一个打印作业。
type Job struct {
	ID        string  `json:"id"`
	User      string  `json:"user"`
	Pages     []int   `json:"pages"`
	Copies    int     `json:"copies"`
	Duplex    bool    `json:"duplex"`
	Sheets    []Sheet `json:"sheets"`
	Confirmed int     `json:"confirmed"` // 已确认的纸张数（前缀）
	Paused    bool    `json:"paused"`
	Status    string  `json:"status"`
	Seq       int64   `json:"seq"` // 提交顺序
}

// User 记录额度与用纸核算。
type User struct {
	Name     string `json:"name"`
	Quota    int    `json:"quota"`    // 总额度（纸张数）
	Used     int    `json:"used"`     // 已实际消耗
	Reserved int    `json:"reserved"` // 尚未打印的预留
}

// Spool 是内存中的全部持久化状态。
type Spool struct {
	Users map[string]*User `json:"users"`
	Jobs  []*Job           `json:"jobs"`
	Seq   int64            `json:"seq"`
}

func NewSpool() *Spool {
	return &Spool{Users: map[string]*User{}}
}

func (s *Spool) findJob(id string) *Job {
	for _, j := range s.Jobs {
		if j.ID == id {
			return j
		}
	}
	return nil
}

// Register 登记或调整用户额度。
func (s *Spool) Register(name string, quota int) error {
	if name == "" || quota < 0 {
		return fmt.Errorf("非法的用户或额度")
	}
	u := s.Users[name]
	if u == nil {
		s.Users[name] = &User{Name: name, Quota: quota}
		return nil
	}
	if quota < u.Used+u.Reserved {
		return fmt.Errorf("新额度低于已用加预留，拒绝调整")
	}
	u.Quota = quota
	return nil
}

// Submit 校验并入队作业，预留整个计划的纸张。
func (s *Spool) Submit(user, id string, totalPages int, ranges string, copies int, duplex bool) (*Job, error) {
	u := s.Users[user]
	if u == nil {
		return nil, fmt.Errorf("用户 %q 未登记", user)
	}
	if id == "" {
		return nil, fmt.Errorf("作业 ID 不能为空")
	}
	if s.findJob(id) != nil {
		return nil, fmt.Errorf("作业 %q 已存在", id)
	}
	pages, err := parseRanges(ranges, totalPages)
	if err != nil {
		return nil, err
	}
	sheets, err := buildSheets(pages, copies, duplex)
	if err != nil {
		return nil, err
	}
	need := len(sheets)
	if u.Used+u.Reserved+need > u.Quota {
		return nil, fmt.Errorf("额度不足：需要 %d 张，可用 %d 张", need, u.Quota-u.Used-u.Reserved)
	}
	u.Reserved += need
	s.Seq++
	job := &Job{
		ID: id, User: user, Pages: pages, Copies: copies, Duplex: duplex,
		Sheets: sheets, Status: StatusPending, Seq: s.Seq,
	}
	s.Jobs = append(s.Jobs, job)
	return job, nil
}

// activeJob 返回当前应处理的作业：正在打印中的作业优先（不抢占），
// 否则取提交最早、未结束且未暂挂的作业。
func (s *Spool) activeJob() *Job {
	var best *Job
	for _, j := range s.Jobs {
		if j.Status == StatusDone || j.Status == StatusCanceled || j.Paused {
			continue
		}
		if j.Status == StatusPrinting {
			return j
		}
		if best == nil || j.Seq < best.Seq {
			best = j
		}
	}
	return best
}

// Pause 暂挂一个尚未开始的作业。
func (s *Spool) Pause(id string) error {
	j := s.findJob(id)
	if j == nil {
		return fmt.Errorf("作业 %q 不存在", id)
	}
	if j.Status != StatusPending {
		return fmt.Errorf("只能暂挂未开始的作业，当前状态 %s", j.Status)
	}
	if j.Paused {
		return fmt.Errorf("作业 %q 已处于暂挂", id)
	}
	j.Paused = true
	return nil
}

// Resume 恢复暂挂的作业。
func (s *Spool) Resume(id string) error {
	j := s.findJob(id)
	if j == nil {
		return fmt.Errorf("作业 %q 不存在", id)
	}
	if j.Status != StatusPending || !j.Paused {
		return fmt.Errorf("作业 %q 不在暂挂状态", id)
	}
	j.Paused = false
	return nil
}

// Confirm 确认某作业的一张纸已打印，把对应预留转为实际消耗。
// sheet 为 0 基纸张序号：重复确认已确认的纸幂等返回；
// 跳过未确认纸张或确认非当前活动作业都会被拒绝。
func (s *Spool) Confirm(id string, sheet int) error {
	j := s.findJob(id)
	if j == nil {
		return fmt.Errorf("作业 %q 不存在", id)
	}
	if j.Status == StatusDone || j.Status == StatusCanceled {
		return fmt.Errorf("作业 %q 已结束，不能继续确认", id)
	}
	if sheet < 0 || sheet >= len(j.Sheets) {
		return fmt.Errorf("纸张序号 %d 超出范围", sheet)
	}
	if sheet < j.Confirmed {
		return nil // 重复确认，不重复扣纸
	}
	if sheet > j.Confirmed {
		return fmt.Errorf("必须先确认第 %d 张，不允许跳过", j.Confirmed)
	}
	active := s.activeJob()
	if active != j {
		return fmt.Errorf("作业 %q 不是当前活动作业", id)
	}
	j.Status = StatusPrinting
	j.Confirmed++
	u := s.Users[j.User]
	u.Reserved--
	u.Used++
	if j.Confirmed == len(j.Sheets) {
		j.Status = StatusDone
	}
	return nil
}

// Cancel 取消作业：只释放尚未打印的预留，保留已消耗纸张与历史。
func (s *Spool) Cancel(id string) error {
	j := s.findJob(id)
	if j == nil {
		return fmt.Errorf("作业 %q 不存在", id)
	}
	if j.Status == StatusDone || j.Status == StatusCanceled {
		return fmt.Errorf("作业 %q 已结束，不能取消", id)
	}
	remaining := len(j.Sheets) - j.Confirmed
	u := s.Users[j.User]
	u.Reserved -= remaining
	j.Status = StatusCanceled
	j.Paused = false
	return nil
}
