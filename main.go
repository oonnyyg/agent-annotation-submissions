package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

// Command 是一条 JSONL 输入命令。
type Command struct {
	Cmd        string `json:"cmd"`
	User       string `json:"user,omitempty"`
	Quota      int    `json:"quota,omitempty"`
	Job        string `json:"job,omitempty"`
	TotalPages int    `json:"pages,omitempty"`
	Ranges     string `json:"ranges,omitempty"`
	Copies     int    `json:"copies,omitempty"`
	Duplex     bool   `json:"duplex,omitempty"`
	Sheet      int    `json:"sheet,omitempty"`
}

// Response 是一条 JSONL 输出。
type Response struct {
	OK    bool        `json:"ok"`
	Error string      `json:"error,omitempty"`
	Data  interface{} `json:"data,omitempty"`
}

func main() {
	statePath := flag.String("state", "spool_state.json", "持久化状态文件路径")
	flag.Parse()

	spool, err := loadSpool(*statePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "加载状态失败:", err)
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		resp := handleLine(spool, line)
		if resp.OK {
			if err := saveSpool(*statePath, spool); err != nil {
				resp = Response{OK: false, Error: "持久化失败: " + err.Error()}
			}
		}
		enc.Encode(resp)
	}
}

func handleLine(spool *Spool, line []byte) Response {
	var c Command
	if err := json.Unmarshal(line, &c); err != nil {
		return Response{OK: false, Error: "非法 JSON: " + err.Error()}
	}
	switch c.Cmd {
	case "register":
		if err := spool.Register(c.User, c.Quota); err != nil {
			return Response{OK: false, Error: err.Error()}
		}
		return Response{OK: true, Data: spool.Users[c.User]}
	case "submit":
		job, err := spool.Submit(c.User, c.Job, c.TotalPages, c.Ranges, c.Copies, c.Duplex)
		if err != nil {
			return Response{OK: false, Error: err.Error()}
		}
		return Response{OK: true, Data: job}
	case "pause":
		return okOrErr(spool.Pause(c.Job))
	case "resume":
		return okOrErr(spool.Resume(c.Job))
	case "confirm":
		return okOrErr(spool.Confirm(c.Job, c.Sheet))
	case "cancel":
		return okOrErr(spool.Cancel(c.Job))
	case "plan":
		j := spool.findJob(c.Job)
		if j == nil {
			return Response{OK: false, Error: fmt.Sprintf("作业 %q 不存在", c.Job)}
		}
		return Response{OK: true, Data: j.Sheets}
	case "queue":
		return Response{OK: true, Data: queueView(spool)}
	case "report":
		if c.User != "" {
			u := spool.Users[c.User]
			if u == nil {
				return Response{OK: false, Error: fmt.Sprintf("用户 %q 未登记", c.User)}
			}
			return Response{OK: true, Data: u}
		}
		users := make([]*User, 0, len(spool.Users))
		for _, u := range spool.Users {
			users = append(users, u)
		}
		return Response{OK: true, Data: users}
	default:
		return Response{OK: false, Error: fmt.Sprintf("未知命令 %q", c.Cmd)}
	}
}

func okOrErr(err error) Response {
	if err != nil {
		return Response{OK: false, Error: err.Error()}
	}
	return Response{OK: true}
}

// QueueItem 是队列状态视图中的一个作业。
type QueueItem struct {
	Job       string `json:"job"`
	User      string `json:"user"`
	Status    string `json:"status"`
	Paused    bool   `json:"paused"`
	Confirmed int    `json:"confirmed"`
	Sheets    int    `json:"sheets"`
	Active    bool   `json:"active"`
}

func queueView(spool *Spool) []QueueItem {
	active := spool.activeJob()
	items := make([]QueueItem, 0, len(spool.Jobs))
	for _, j := range spool.Jobs {
		items = append(items, QueueItem{
			Job: j.ID, User: j.User, Status: j.Status, Paused: j.Paused,
			Confirmed: j.Confirmed, Sheets: len(j.Sheets), Active: j == active,
		})
	}
	return items
}

func loadSpool(path string) (*Spool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return NewSpool(), nil
	}
	if err != nil {
		return nil, err
	}
	spool := NewSpool()
	if err := json.Unmarshal(data, spool); err != nil {
		return nil, err
	}
	if spool.Users == nil {
		spool.Users = map[string]*User{}
	}
	return spool, nil
}

func saveSpool(path string, spool *Spool) error {
	data, err := json.MarshalIndent(spool, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
