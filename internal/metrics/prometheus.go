package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"k8s.io/client-go/rest"
)

type PrometheusProvider struct {
	baseURL    string
	httpClient *http.Client
}

func NewPrometheusProvider(baseURL string, restConfig *rest.Config) (*PrometheusProvider, error) {
	baseURL = strings.TrimRight(baseURL, "/")

	var transport http.RoundTripper
	if restConfig != nil {
		t, err := rest.TransportFor(restConfig)
		if err != nil {
			return nil, fmt.Errorf("building transport from rest config: %w", err)
		}
		transport = t
	} else {
		transport = &http.Transport{
			DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		}
	}

	return &PrometheusProvider{
		baseURL: baseURL,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}, nil
}

func (p *PrometheusProvider) Name() string { return "prometheus" }

func (p *PrometheusProvider) Available() bool {
	return p.probe() == nil
}

// probe checks whether the Prometheus endpoint is reachable and returns the
// reason if it is not. Tries /-/ready first, then /api/v1/status/config.
func (p *PrometheusProvider) probe() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/-/ready", nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET /-/ready: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	readyStatus := resp.StatusCode

	// Fall back to config endpoint
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/api/v1/status/config", nil)
	if err != nil {
		return fmt.Errorf("building fallback request: %w", err)
	}
	resp, err = p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET /api/v1/status/config: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	return fmt.Errorf("/-/ready returned HTTP %d, /api/v1/status/config returned HTTP %d", readyStatus, resp.StatusCode)
}

func (p *PrometheusProvider) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]TimeSeries, error) {
	params := url.Values{
		"query": {query},
		"start": {strconv.FormatInt(start.Unix(), 10)},
		"end":   {strconv.FormatInt(end.Unix(), 10)},
		"step":  {strconv.FormatFloat(step.Seconds(), 'f', -1, 64)},
	}

	body, err := p.doGet(ctx, "/api/v1/query_range", params)
	if err != nil {
		return nil, err
	}
	return parseMatrixResponse(body)
}

func (p *PrometheusProvider) QueryInstant(ctx context.Context, resourceType string, namespace string, name string) (*MetricsResponse, error) {
	queries, ok := BuiltinQueries[resourceType]
	if !ok {
		return nil, fmt.Errorf("no built-in queries for resource type %q", resourceType)
	}

	vars := map[string]string{
		"namespace": namespace,
		"name":      name,
	}

	var results []MetricResult
	for _, mq := range queries {
		promQL := SubstituteVars(mq.Query, vars)
		series, err := p.queryInstantRaw(ctx, promQL)
		if err != nil {
			return nil, fmt.Errorf("instant query %q: %w", mq.Name, err)
		}
		results = append(results, MetricResult{
			Name:   mq.Name,
			Unit:   mq.Unit,
			Series: series,
		})
	}

	return &MetricsResponse{Metrics: results}, nil
}

func (p *PrometheusProvider) queryInstantRaw(ctx context.Context, query string) ([]TimeSeries, error) {
	params := url.Values{"query": {query}}
	body, err := p.doGet(ctx, "/api/v1/query", params)
	if err != nil {
		return nil, err
	}
	return parseVectorResponse(body)
}

// RawQueryInstant executes a raw PromQL instant query and returns the vector result.
func (p *PrometheusProvider) RawQueryInstant(ctx context.Context, query string) ([]TimeSeries, error) {
	return p.queryInstantRaw(ctx, query)
}

func (p *PrometheusProvider) doGet(ctx context.Context, path string, params url.Values) ([]byte, error) {
	u := p.baseURL + path + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var promErr prometheusErrorResponse
		if json.Unmarshal(body, &promErr) == nil && promErr.Error != "" {
			return nil, fmt.Errorf("prometheus %s (HTTP %d): %s", promErr.ErrorType, resp.StatusCode, promErr.Error)
		}
		return nil, fmt.Errorf("prometheus HTTP %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// Prometheus API response structures

type prometheusErrorResponse struct {
	Status    string `json:"status"`
	ErrorType string `json:"errorType"`
	Error     string `json:"error"`
}

type prometheusResponse struct {
	Status string          `json:"status"`
	Data   json.RawMessage `json:"data"`
}

type prometheusData struct {
	ResultType string            `json:"resultType"`
	Result     []json.RawMessage `json:"result"`
}

type matrixResult struct {
	Metric map[string]string   `json:"metric"`
	Values [][]json.RawMessage `json:"values"`
}

type vectorResult struct {
	Metric map[string]string `json:"metric"`
	Value  []json.RawMessage `json:"value"`
}

func parseMatrixResponse(body []byte) ([]TimeSeries, error) {
	var resp prometheusResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing prometheus response: %w", err)
	}
	if resp.Status != "success" {
		return nil, fmt.Errorf("prometheus query failed: status=%s", resp.Status)
	}

	var data prometheusData
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return nil, fmt.Errorf("parsing data field: %w", err)
	}
	if data.ResultType != "matrix" {
		return nil, fmt.Errorf("expected matrix result type, got %q", data.ResultType)
	}

	var series []TimeSeries
	for _, raw := range data.Result {
		var mr matrixResult
		if err := json.Unmarshal(raw, &mr); err != nil {
			return nil, fmt.Errorf("parsing matrix result: %w", err)
		}

		ts := TimeSeries{Labels: mr.Metric}
		for _, pair := range mr.Values {
			if len(pair) != 2 {
				continue
			}
			pt, err := parseSamplePair(pair[0], pair[1])
			if err != nil {
				return nil, err
			}
			ts.Points = append(ts.Points, pt)
		}
		series = append(series, ts)
	}
	return series, nil
}

func parseVectorResponse(body []byte) ([]TimeSeries, error) {
	var resp prometheusResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing prometheus response: %w", err)
	}
	if resp.Status != "success" {
		return nil, fmt.Errorf("prometheus query failed: status=%s", resp.Status)
	}

	var data prometheusData
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return nil, fmt.Errorf("parsing data field: %w", err)
	}
	if data.ResultType != "vector" {
		return nil, fmt.Errorf("expected vector result type, got %q", data.ResultType)
	}

	var series []TimeSeries
	for _, raw := range data.Result {
		var vr vectorResult
		if err := json.Unmarshal(raw, &vr); err != nil {
			return nil, fmt.Errorf("parsing vector result: %w", err)
		}
		if len(vr.Value) != 2 {
			continue
		}
		pt, err := parseSamplePair(vr.Value[0], vr.Value[1])
		if err != nil {
			return nil, err
		}
		series = append(series, TimeSeries{
			Labels: vr.Metric,
			Points: []TimeSeriesPoint{pt},
		})
	}
	return series, nil
}

func parseSamplePair(tsRaw, valRaw json.RawMessage) (TimeSeriesPoint, error) {
	var ts float64
	if err := json.Unmarshal(tsRaw, &ts); err != nil {
		return TimeSeriesPoint{}, fmt.Errorf("parsing timestamp: %w", err)
	}

	var valStr string
	if err := json.Unmarshal(valRaw, &valStr); err != nil {
		return TimeSeriesPoint{}, fmt.Errorf("parsing value: %w", err)
	}
	val, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		return TimeSeriesPoint{}, fmt.Errorf("converting value %q to float: %w", valStr, err)
	}

	return TimeSeriesPoint{
		Timestamp: int64(ts),
		Value:     val,
	}, nil
}
