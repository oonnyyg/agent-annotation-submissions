package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// 目标状态。
const (
	StatusCached   = "cached"   // 缓存命中，未执行
	StatusRebuilt  = "rebuilt"  // 实际执行并成功
	StatusFailed   = "failed"   // 执行失败
	StatusSkipped  = "skipped"  // 因依赖失败而跳过
	StatusPlanRun  = "would-run"
	StatusPlanHit  = "would-cache"
	StatusPlanWait = "pending-upstream" // 上游将重建，结果待上游输出决定
)

// TargetReport 是单个目标的执行报告。
type TargetReport struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Reason string `json:"reason"`
	Log    string `json:"log,omitempty"`
}

// Report 是整体执行报告。
type Report struct {
	DryRun  bool           `json:"dry_run"`
	Targets []TargetReport `json:"targets"`
	Failed  bool           `json:"failed"`
}

type runner struct {
	manifest *Manifest
	store    *cacheStore
	logDir   string
	dryRun   bool
	reports  map[string]*TargetReport
}

func newRunner(m *Manifest, store *cacheStore, dryRun bool) *runner {
	return &runner{
		manifest: m,
		store:    store,
		logDir:   filepath.Join(store.dir, "logs"),
		dryRun:   dryRun,
		reports:  map[string]*TargetReport{},
	}
}

// run 按给定顺序处理每个目标，返回报告。
func (r *runner) run(order []string) *Report {
	rep := &Report{DryRun: r.dryRun}
	for _, name := range order {
		tr := r.runOne(name)
		r.reports[name] = tr
		rep.Targets = append(rep.Targets, *tr)
		if tr.Status == StatusFailed {
			rep.Failed = true
		}
	}
	return rep
}

func (r *runner) runOne(name string) *TargetReport {
	t := r.manifest.Targets[name]
	// 依赖失败/跳过/计划重建时，本目标不能给出确定结论。
	for _, d := range t.Deps {
		dr := r.reports[d]
		if dr == nil {
			continue // 依赖不在本次闭包内（防御，正常不会发生）
		}
		switch dr.Status {
		case StatusFailed, StatusSkipped:
			return &TargetReport{Name: name, Status: StatusSkipped,
				Reason: fmt.Sprintf("依赖 %q %s，跳过", d, dr.Status)}
		case StatusPlanRun, StatusPlanWait:
			return &TargetReport{Name: name, Status: StatusPlanWait,
				Reason: fmt.Sprintf("上游 %q 计划重建，新输出指纹待定", d)}
		}
	}
	fp, err := r.manifest.fingerprint(t)
	if err != nil {
		if r.dryRun {
			// 预览阶段输入可能尚不存在（将由上游生成），无法判定。
			return &TargetReport{Name: name, Status: StatusPlanWait,
				Reason: fmt.Sprintf("无法计算指纹: %v", err)}
		}
		return &TargetReport{Name: name, Status: StatusFailed, Reason: err.Error()}
	}
	hit, reason := r.manifest.checkHit(r.store, t, fp)
	if r.dryRun {
		if hit {
			return &TargetReport{Name: name, Status: StatusPlanHit, Reason: reason}
		}
		return &TargetReport{Name: name, Status: StatusPlanRun, Reason: reason}
	}
	if hit {
		return &TargetReport{Name: name, Status: StatusCached, Reason: reason}
	}
	return r.execute(t, fp, reason)
}

// execute 直接按参数数组启动命令，固定工作目录为工作根。
func (r *runner) execute(t *Target, fp, missReason string) *TargetReport {
	if err := os.MkdirAll(r.logDir, 0o755); err != nil {
		return &TargetReport{Name: t.Name, Status: StatusFailed, Reason: err.Error()}
	}
	logPath := filepath.Join(r.logDir, t.Name+".log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return &TargetReport{Name: t.Name, Status: StatusFailed, Reason: err.Error()}
	}
	defer logFile.Close()
	fmt.Fprintf(logFile, "重建原因: %s\n命令: %v\n\n", missReason, t.Command)

	cmd := exec.Command(t.Command[0], t.Command[1:]...)
	cmd.Dir = r.manifest.Root
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	rep := &TargetReport{Name: t.Name, Log: logPath}
	if err := cmd.Run(); err != nil {
		rep.Status = StatusFailed
		rep.Reason = fmt.Sprintf("%s；命令非零退出或无法启动: %v", missReason, err)
		return rep
	}
	hashes, err := r.manifest.hashOutputs(t)
	if err != nil {
		rep.Status = StatusFailed
		rep.Reason = fmt.Sprintf("%s；命令成功但%v", missReason, err)
		return rep
	}
	r.store.Entries[t.Name] = &cacheEntry{Fingerprint: fp, OutputHashes: hashes}
	if err := r.store.save(); err != nil {
		rep.Status = StatusFailed
		rep.Reason = fmt.Sprintf("写入缓存失败: %v", err)
		return rep
	}
	rep.Status = StatusRebuilt
	rep.Reason = missReason
	return rep
}
