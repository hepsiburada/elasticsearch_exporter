package collector

type Header struct {
}

type Task struct {
	Node               string            `json:"node"`
	Id                 int               `json:"id"`
	Type               string            `json:"type"`
	Action             string            `json:"action"`
	StartTimeInMillis  int               `json:"start_time_in_millis"`
	RunningTimeInNanos int               `json:"running_time_in_nanos"`
	Cancellable        bool              `json:"cancellable"`
	ParentTaskId       string            `json:"parent_task_id"`
	Headers            map[string]Header `json:"headers"`
}

type Node struct {
	Name             string          `json:"name"`
	TransportAddress string          `json:"transport_address"`
	Host             string          `json:"host"`
	Ip               string          `json:"ip"`
	Roles            []string        `json:"roles"`
	Tasks            map[string]Task `json:"tasks"`
}

type TasksResponse struct {
	Nodes map[string]Node `json:"nodes"`
}
