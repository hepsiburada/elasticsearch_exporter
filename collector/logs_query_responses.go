package collector

type Hits struct {
	Total int `json:"total"`
}

type NetworkDiscoveryErrorQueryResponse struct {
	Hits Hits `json:"hits"`
}
