/*
Package metrics provides setup of metrics that can be used internally to measure various application states.
All metrics for the application are defined here and other applications use this package to grab the metrics
and use them. This package will also report any metric that is not used in the first 10 seconds after the app has started
to prevent useless metrics from existing, as all metrics should be grabbed by that time.

In a package you want to set metrics, you can do it as follows:

	var addCount metrics.Int64Counter

	func init() {
		addCounter = metrics.Get.Int64("petstore/server/AddPets/requests")
	}
	...

	func (s *Server) AddPets(ctx context.Context, req *pb.AddPetsReq) (*pb.AddpetsResp, error) {
		...
		// Do this if you have multiple changes that don't require special labels per update.
		metrics.Meter.RecordBatch(ctx, nil, addCounter.Measure(ctx, 1))
		// Do this if you only need to make one change or need special labels.
		addCounter.Add(ctx, 1, attribute.String("label", "value")
		...
	}

To cause metrics to be exported package main():

	func main() {
		...
		stop, err := metrics.Start(ctx, metrics.OTELGRPC{Addr: "ip:port"})
		if err != nil {
			log.Fatal(err)
		}
		defer stop()
		...
	}
*/
package metrics

import (
	"go.opentelemetry.io/otel"
	"html/template"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gc-2023/kubernetes/petstore/server/log"

	"go.opentelemetry.io/otel/metric"
)

type metricType int

const (
	unknown     = 0
	mtInt64     = 1
	mtInt64Hist = 2
	mtInt64UD   = 3
)

type metricDef struct {
	mtype metricType
	name  string
	desc  string
}

var metrics = []metricDef{
	// Histograms
	{mtInt64Hist, "AddPets-latency", "The latency of an AddPets() request in nanoseconds"},
	{mtInt64Hist, "DeletePets-latency", "The latency of an DeletePets() request in nanoseconds"},
	{mtInt64Hist, "UpdatePets-latency", "The latency of an UpdatePets() request in nanoseconds"},
	{mtInt64Hist, "SearchPets-latency", "The latency of a SearchPets() request in nanoseconds"},
	// Counters
	{mtInt64, "AddPets-requests", "The total requests made to AddPets()"},
	{mtInt64, "DeletePets-requests", "The total requests made to DeletePets()"},
	{mtInt64, "UpdatePets-requests", "The total requests made to UpdatePets()"},
	{mtInt64, "SearchPets-requests", "The total requests made to SearchPets()"},
	{mtInt64, "totals-requests", "The total requests made to the server"},

	{mtInt64, "AddPets-errors", "The total error count"},
	{mtInt64, "DeletePets-errors", "The total error couunt"},
	{mtInt64, "UpdatePets-errors", "The total error count"},
	{mtInt64, "SearchPets-errors", "The total error count"},
	{mtInt64, "totals-errors", "The total error count for all RPCs"},

	// UpDown Counters
	{mtInt64UD, "AddPets-current", "The amount of requests currently being proccessed"},
	{mtInt64UD, "DeletePets-current", "The amount of requests currently being proccessed"},
	{mtInt64UD, "UpdatePets-current", "The amount of requests currently being proccessed"},
	{mtInt64UD, "SearchPets-current", "The amount of requests currently being proccessed"},
}

// Meter is the meter for the petstore.
var Meter = otel.GetMeterProvider().Meter("petstore")

// Get is used to lookup metrics by name.
var Get = newLookups()

var unusedMetricsTmpl = template.Must(
	template.New("").Parse(
		`
The following metrics appeart to be unused:
{{- range .}}
	{{.}}
{{- end }}
`,
	),
)

// Lookups provides lookups for metrics based on their names.
type Lookups struct {
	mtInt64Hist map[string]metric.Int64Histogram
	mtInt64UD   map[string]metric.Int64UpDownCounter
	mtInt64     map[string]metric.Int64Counter

	mu   sync.Mutex
	used map[string]bool
}

func newLookups() *Lookups {
	l := &Lookups{
		mtInt64Hist: map[string]metric.Int64Histogram{},
		mtInt64:     map[string]metric.Int64Counter{},
		mtInt64UD:   map[string]metric.Int64UpDownCounter{},
		used:        map[string]bool{},
	}

	exists := map[string]bool{}
	for _, m := range metrics {
		if m.mtype == unknown {
			log.Logger.Fatalf("metric with type(%v) cannot be added", m.mtype)
		}
		if m.name == "" {
			log.Logger.Fatalf("metric cannot be missing a name")
		}
		if m.desc == "" {
			log.Logger.Fatalf("metric cannot be missing a desc")
		}
		if exists[m.name] {
			log.Logger.Fatalf("cannot have two metrics with same name(%s)", m.name)
		}
		exists[m.name] = true

		switch m.mtype {
		case mtInt64Hist:
			l.mtInt64Hist[m.name] = Must(Meter.Int64Histogram(m.name, metric.WithDescription(m.desc)))
		case mtInt64UD:
			l.mtInt64UD[m.name] = Must(Meter.Int64UpDownCounter(m.name, metric.WithDescription(m.desc)))
		case mtInt64:
			l.mtInt64[m.name] = Must(Meter.Int64Counter(m.name, metric.WithDescription(m.desc)))
		default:
			log.Logger.Fatalf("bug: we defined a metric type(%v) without adding support", m.mtype)
		}
	}
	go func() {
		time.Sleep(10 * time.Second)
		unused := l.unused()
		s := strings.Builder{}
		if err := unusedMetricsTmpl.Execute(&s, unused); err != nil {
			log.Logger.Fatalf("unusedMetricTmpl execute error: %s", err)
		}
		log.Logger.Println(s.String())
	}()
	return l
}

// Int64 grabs the Int64Counter metric named "s". If not found, panics.
func (l *Lookups) Int64(s string) metric.Int64Counter {
	l.mu.Lock()
	defer l.mu.Unlock()

	m, ok := l.mtInt64[s]
	if !ok {
		log.Logger.Fatalf("int64 metric(%s) is not defined", s)
	}
	l.used[s] = true
	return m
}

// Int64s grabs a list of Int64Counters.
func (l *Lookups) Int64s(s ...string) []metric.Int64Counter {
	v := make([]metric.Int64Counter, 0, len(s))
	for _, name := range s {
		v = append(v, l.Int64(name))
	}
	return v
}

// Int64UD grabs the Int64UpDownCounter metric named "s". If not found, panics.
func (l *Lookups) Int64UD(s string) metric.Int64UpDownCounter {
	l.mu.Lock()
	defer l.mu.Unlock()

	m, ok := l.mtInt64UD[s]
	if !ok {
		log.Logger.Fatalf("int64ud metric(%s) is not defined", s)
	}
	l.used[s] = true
	return m
}

// Int64UDs grabs a list of Int64UpDownCounters.
func (l *Lookups) Int64UDs(s ...string) []metric.Int64UpDownCounter {
	v := make([]metric.Int64UpDownCounter, 0, len(s))
	for _, name := range s {
		v = append(v, l.Int64UD(name))
	}
	return v
}

// Int64Hist grabs the Int64Histogram metric named "s". If not found, panics.
func (l *Lookups) Int64Hist(s string) metric.Int64Histogram {
	l.mu.Lock()
	defer l.mu.Unlock()

	m, ok := l.mtInt64Hist[s]
	if !ok {
		log.Logger.Fatalf("int64 histogram metric(%s) is not defined", s)
	}
	l.used[s] = true
	return m
}

func (l *Lookups) Int64Hists(s ...string) []metric.Int64Histogram {
	v := make([]metric.Int64Histogram, 0, len(s))
	for _, name := range s {
		v = append(v, l.Int64Hist(name))
	}
	return v
}

func (l *Lookups) unused() []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	unused := []string{}
	for k := range l.mtInt64Hist {
		if !l.used[k] {
			unused = append(unused, k)
		}
	}
	for k := range l.mtInt64 {
		if !l.used[k] {
			unused = append(unused, k)
		}
	}
	sort.Strings(unused)
	return unused
}

func Must[T any](m T, err error) T {
	if err != nil {
		log.Logger.Fatalf("failed to create meter: %s", err)
	}
	return m
}
