package collector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
	"net/url"
	"path"
)

type networkDiscoveryErrorQueryMetric struct {
	Type  prometheus.ValueType
	Desc  *prometheus.Desc
	Value func(response NetworkDiscoveryErrorQueryResponse) float64
}

type LogsQueries struct {
	logger log.Logger
	client *http.Client
	url    *url.URL

	up                              prometheus.Gauge
	totalScrapes, jsonParseFailures prometheus.Counter

	networkDiscoveryErrorQueryMetrics []*networkDiscoveryErrorQueryMetric
}

func NewLogsQueries(logger log.Logger, client *http.Client, url *url.URL) *LogsQueries {
	subsystem := "queries"

	return &LogsQueries{
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

		networkDiscoveryErrorQueryMetrics: []*networkDiscoveryErrorQueryMetric{
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, subsystem, "total_network_discovery_error"),
					"Number of total network discovery",
					nil, nil,
				),
				Value: func(response NetworkDiscoveryErrorQueryResponse) float64 {
					return float64(response.Hits.Total)
				},
			},
		},
	}
}

func (s *LogsQueries) Describe(ch chan<- *prometheus.Desc) {
	for _, metric := range s.networkDiscoveryErrorQueryMetrics {
		ch <- metric.Desc
	}
	ch <- s.up.Desc()
	ch <- s.totalScrapes.Desc()
	ch <- s.jsonParseFailures.Desc()
}

func (s *LogsQueries) getNetworkDiscoveryReq(u *url.URL) (*http.Response, error) {
	var jsonStr = []byte(`{
	"query": {
		"bool": {
			"must": [{
				"match_all": {}
			}, {
				"bool": {
					"should": [{
						"match_phrase": {
							"message": "send message failed"
						}
					}, {
						"match_phrase": {
							"message": "NodeNotConnectedException"
						}
					}]
				}
			}, {
				"range": {
					"@timestamp": {
						"gt" :  "now-5m",
						"format": "epoch_millis"
					}
				}
			}],
			"must_not": [{
				"match_phrase": {
					"message": {
						"query": "0.0.0.0"
					}
				}
			}]
		}
	}
}`)
	req, _ := http.NewRequest("POST", u.String(), bytes.NewBuffer(jsonStr))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	return client.Do(req)
}

func (s *LogsQueries) getAndParseURL(u *url.URL, data interface{}) error {
	res, err := s.getNetworkDiscoveryReq(u)
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

func (s *LogsQueries) fetchAndDecodeTasks() (NetworkDiscoveryErrorQueryResponse, error) {
	u := *s.url
	u.Path = path.Join(u.Path, "/elasticsearch-*/_search")
	var srr NetworkDiscoveryErrorQueryResponse
	err := s.getAndParseURL(&u, &srr)

	if err != nil {
		return srr, err
	}

	return srr, nil
}

func (s *LogsQueries) Collect(ch chan<- prometheus.Metric) {
	var err error
	s.totalScrapes.Inc()
	defer func() {
		ch <- s.up
		ch <- s.totalScrapes
		ch <- s.jsonParseFailures
	}()

	networkDiscoveryResp, err := s.fetchAndDecodeTasks()
	if err != nil {
		s.up.Set(0)
		_ = level.Warn(s.logger).Log(
			"msg", "failed to fetch and decode tasks",
			"err", err,
		)
		return
	}
	s.up.Set(1)

	for _, metric := range s.networkDiscoveryErrorQueryMetrics {
		ch <- prometheus.MustNewConstMetric(
			metric.Desc,
			metric.Type,
			metric.Value(networkDiscoveryResp),
		)
	}
}
