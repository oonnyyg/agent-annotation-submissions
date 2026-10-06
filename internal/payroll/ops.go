package payroll

import (
	"errors"
	"fmt"
)

// Handle 执行一个操作。只有成功且产生状态变更的操作
// 才会落盘；失败不留下任何部分状态。
func (s *Service) Handle(req Request) Response {
	resp := Response{Op: req.Op}
	var err error
	switch req.Op {
	case "register_scheme":
		err = s.registerScheme(req)
	case "register_batch":
		resp.Shares, resp.Total, err = s.registerBatch(req)
		resp.BatchID = req.BatchID
	case "void_batch":
		resp.Shares, err = s.voidBatch(req)
		resp.BatchID = req.BatchID
	case "close_month":
		err = s.closeMonth(req)
	case "query_month":
		resp.Report, err = s.queryMonth(req)
	default:
		err = fmt.Errorf("unknown op %q", req.Op)
	}
	if err != nil {
		resp.OK = false
		resp.Error = err.Error()
		return resp
	}
	resp.OK = true
	return resp
}

func (s *Service) registerScheme(req Request) error {
	if req.SchemeID == "" {
		return errors.New("scheme_id required")
	}
	if req.Version <= 0 {
		return errors.New("version must be positive")
	}
	if len(req.Tiers) == 0 {
		return errors.New("tiers required")
	}
	for _, v := range s.state.Schemes[req.SchemeID] {
		if v.Version == req.Version {
			return fmt.Errorf("scheme %s version %d already registered", req.SchemeID, req.Version)
		}
	}
	sv := SchemeVersion{SchemeID: req.SchemeID, Version: req.Version, Tiers: req.Tiers}
	s.state.Schemes[req.SchemeID] = append(s.state.Schemes[req.SchemeID], sv)
	return s.save()
}

func (s *Service) schemeVersion(schemeID string, version int) *SchemeVersion {
	for i := range s.state.Schemes[schemeID] {
		if s.state.Schemes[schemeID][i].Version == version {
			return &s.state.Schemes[schemeID][i]
		}
	}
	return nil
}

func (s *Service) latestScheme(schemeID string) *SchemeVersion {
	vs := s.state.Schemes[schemeID]
	if len(vs) == 0 {
		return nil
	}
	latest := &vs[0]
	for i := range vs {
		if vs[i].Version > latest.Version {
			latest = &vs[i]
		}
	}
	return latest
}
