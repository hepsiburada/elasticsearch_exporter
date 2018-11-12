package collector

import (
	"encoding/json"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
	"net/url"
	"path"
)

type taskMetric struct {
	Type  prometheus.ValueType
	Desc  *prometheus.Desc
	Value func(taskStat TasksResponse) float64
}

type Tasks struct {
	logger log.Logger
	client *http.Client
	url    *url.URL

	up                              prometheus.Gauge
	totalScrapes, jsonParseFailures prometheus.Counter

	taskMetrics []*taskMetric
}

func NewTasks(logger log.Logger, client *http.Client, url *url.URL) *Tasks {
	subsystem := "tasks"

	return &Tasks{
		logger: logger,
		client: client,
		url:    url,

		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: prometheus.BuildFQName(namespace, subsystem, "up"),
			Help: "Was the last scrape of the ElasticSearch tasks endpoint successful.",
		}),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Name: prometheus.BuildFQName(namespace, subsystem, "total_scrapes"),
			Help: "Current total ElasticSearch tasks scrapes.",
		}),
		jsonParseFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: prometheus.BuildFQName(namespace, subsystem, "json_parse_failures"),
			Help: "Number of errors while parsing JSON.",
		}),

		taskMetrics: []*taskMetric{
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, subsystem, "total"),
					"Number of tasks",
					nil, nil,
				),
				Value: func(tasks TasksResponse) float64 {
					var count = 0

					for _, node := range tasks.Nodes {
						count += len(node.Tasks)
					}

					return float64(count)
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, subsystem, "total_gt_1s"),
					"Number of tasks greater then 1 second",
					nil, nil,
				),
				Value: func(tasks TasksResponse) float64 {
					var count = 0

					for _, node := range tasks.Nodes {
						for _, task := range node.Tasks {
							var ms = task.RunningTimeInNanos / 1000000
							if ms > 1000 {
								count += 1
							}
						}
					}

					return float64(count)
				},
			},
		},
	}
}

func (s *Tasks) Describe(ch chan<- *prometheus.Desc) {
	for _, metric := range s.taskMetrics {
		ch <- metric.Desc
	}
	ch <- s.up.Desc()
	ch <- s.totalScrapes.Desc()
	ch <- s.jsonParseFailures.Desc()
}

func (s *Tasks) getAndParseURL(u *url.URL, data interface{}) error {
	res, err := s.client.Get(u.String())
	if err != nil {
		return fmt.Errorf("failed to get from %s://%s:%s%s: %s",
			u.Scheme, u.Hostname(), u.Port(), u.Path, err)
	}

	defer func() {
		err = res.Body.Close()
		if err != nil {
			_ = level.Warn(s.logger).Log(
				"msg", "failed to close http.Client",
				"err", err,
			)
		}
	}()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP Request failed with code %d", res.StatusCode)
	}

	if err := json.NewDecoder(res.Body).Decode(data); err != nil {
		s.jsonParseFailures.Inc()
		return err
	}
	return nil
}

func (s *Tasks) fetchAndDecodeTasks() (TasksResponse, error) {
	u := *s.url
	u.Path = path.Join(u.Path, "/_tasks")
	var srr TasksResponse
	err := s.getAndParseURL(&u, &srr)

	if err != nil {
		return srr, err
	}

	return srr, nil
}

func (s *Tasks) Collect(ch chan<- prometheus.Metric) {
	var err error
	s.totalScrapes.Inc()
	defer func() {
		ch <- s.up
		ch <- s.totalScrapes
		ch <- s.jsonParseFailures
	}()

	tasksResp, err := s.fetchAndDecodeTasks()
	if err != nil {
		s.up.Set(0)
		_ = level.Warn(s.logger).Log(
			"msg", "failed to fetch and decode tasks",
			"err", err,
		)
		return
	}
	s.up.Set(1)

	for _, metric := range s.taskMetrics {
		ch <- prometheus.MustNewConstMetric(
			metric.Desc,
			metric.Type,
			metric.Value(tasksResp),
		)
	}
}
