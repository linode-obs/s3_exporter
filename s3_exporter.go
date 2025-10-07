package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/version"
	kingpin "gopkg.in/alecthomas/kingpin.v2"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
)

const (
	namespace = "s3"
)

// Global configuration variables
var (
	useHeadMethod   bool
	prodSafeMode    bool
	maxObjectsLimit int
)

var (
	s3ListSuccess = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "list_success"),
		"If the ListObjects operation was a success",
		[]string{"bucket", "prefix", "delimiter"}, nil,
	)
	s3ListDuration = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "list_duration_seconds"),
		"The total duration of the list operation",
		[]string{"bucket", "prefix", "delimiter"}, nil,
	)
	s3LastModifiedObjectDate = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "last_modified_object_date"),
		"The last modified date of the object that was modified most recently",
		[]string{"bucket", "prefix"}, nil,
	)
	s3LastModifiedObjectSize = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "last_modified_object_size_bytes"),
		"The size of the object that was modified most recently",
		[]string{"bucket", "prefix"}, nil,
	)
	s3ObjectTotal = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "objects"),
		"The total number of objects for the bucket/prefix combination",
		[]string{"bucket", "prefix"}, nil,
	)
	s3SumSize = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "objects_size_sum_bytes"),
		"The total size of all objects summed",
		[]string{"bucket", "prefix"}, nil,
	)
	s3BiggestObjectSize = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "biggest_object_size_bytes"),
		"The size of the biggest object",
		[]string{"bucket", "prefix"}, nil,
	)
	s3CommonPrefixes = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "common_prefixes"),
		"A count of all the keys between the prefix and the next occurrence of the string specified by the delimiter",
		[]string{"bucket", "prefix", "delimiter"}, nil,
	)
)

type Exporter struct {
	svc           s3iface.S3API
	bucket        string
	prefix        string
	delimiter     string
	useHeadMethod bool
	prodSafeMode  bool
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- s3ListSuccess
	ch <- s3ListDuration
	ch <- s3LastModifiedObjectDate
	ch <- s3LastModifiedObjectSize
	ch <- s3ObjectTotal
	ch <- s3SumSize
	ch <- s3BiggestObjectSize
	ch <- s3CommonPrefixes
}

// getBucketUsageHEAD attempts to query the bucket usage using Ceph-specific HEAD request.
// Ceph RGW returns usage information in the headers (x-rgw-bytes-used, x-rgw-objects-count).
// Reference: https://tracker.ceph.com/issues/2313
func getBucketUsageHEAD(svc s3iface.S3API, bucket string) (float64, float64, error) {
	input := &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	}

	// Send HEAD request to bucket
	req, _ := svc.HeadBucketRequest(input)
	err := req.Send()
	if err != nil {
		return 0, 0, err
	}

	// Extract Ceph RGW specific headers
	headers := req.HTTPResponse.Header
	bytesStr := headers.Get("x-rgw-bytes-used")
	objsStr := headers.Get("x-rgw-objects-count")

	log.Debugf("Received Ceph headers: x-rgw-bytes-used=%q, x-rgw-objects-count=%q", bytesStr, objsStr)

	// Parse bytes used
	var bytesUsed float64 = 0
	if bytesStr != "" {
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(bytesStr), 64); err == nil {
			bytesUsed = parsed
		} else {
			log.Warnf("Failed to parse x-rgw-bytes-used header %q: %v", bytesStr, err)
		}
	}

	// Parse object count
	var objCount float64 = 0
	if objsStr != "" {
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(objsStr), 64); err == nil {
			objCount = parsed
		} else {
			log.Warnf("Failed to parse x-rgw-objects-count header %q: %v", objsStr, err)
		}
	}

	// If we got at least bytes data, consider it successful
	if bytesUsed > 0 || objCount > 0 {
		return bytesUsed, objCount, nil
	}

	return 0, 0, nil
}

// Collect metrics using HEAD method first, falling back to ListObjects if needed
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	// Try HEAD method first for Ceph RGW if enabled
	if e.useHeadMethod {
		bytesUsed, objCount, err := getBucketUsageHEAD(e.svc, e.bucket)
		if err == nil && (bytesUsed > 0 || objCount > 0) {
			// HEAD method succeeded, use those values
			log.Infof("HEAD method successful: %0.0f bytes, %0.0f objects", bytesUsed, objCount)
			ch <- prometheus.MustNewConstMetric(
				s3ListSuccess, prometheus.GaugeValue, 1, e.bucket, e.prefix, e.delimiter,
			)
			ch <- prometheus.MustNewConstMetric(
				s3ObjectTotal, prometheus.GaugeValue, objCount, e.bucket, e.prefix,
			)
			ch <- prometheus.MustNewConstMetric(
				s3SumSize, prometheus.GaugeValue, bytesUsed, e.bucket, e.prefix,
			)
			ch <- prometheus.MustNewConstMetric(
				s3ListDuration, prometheus.GaugeValue, 0.001, e.bucket, e.prefix, e.delimiter,
			)
			return
		}
		log.Warnf("HEAD method failed or returned no data: %v", err)

		// If prod-safe-mode is enabled and HEAD fails, refuse to do dangerous ListObjects
		if e.prodSafeMode {
			log.Errorf("HEAD method failed and prod-safe-mode enabled - refusing ListObjects to prevent incidents")
			ch <- prometheus.MustNewConstMetric(
				s3ListSuccess, prometheus.GaugeValue, 0, e.bucket, e.prefix, e.delimiter,
			)
			return
		}
	}

	// Fallback to traditional ListObjects approach with production safety limits
	e.collectWithListObjects(ch)
}

func (e *Exporter) collectWithListObjects(ch chan<- prometheus.Metric) {
	var lastModified time.Time
	var numberOfObjects float64
	var totalSize int64
	var biggestObjectSize int64
	var lastObjectSize int64
	var commonPrefixes int
	var objectsProcessed int64 = 0

	query := &s3.ListObjectsV2Input{
		Bucket:    aws.String(e.bucket),
		Prefix:    aws.String(e.prefix),
		Delimiter: aws.String(e.delimiter),
	}

	startList := time.Now()
	for {
		resp, err := e.svc.ListObjectsV2(query)
		if err != nil {
			log.Errorln(err)
			ch <- prometheus.MustNewConstMetric(
				s3ListSuccess, prometheus.GaugeValue, 0, e.bucket, e.prefix, e.delimiter,
			)
			return
		}

		commonPrefixes = commonPrefixes + len(resp.CommonPrefixes)

		for _, item := range resp.Contents {
			// Production safety: check object limit
			if objectsProcessed >= int64(maxObjectsLimit) {
				log.Warnf("Reached production safety limit of %d objects, stopping enumeration", maxObjectsLimit)
				break
			}

			numberOfObjects++
			objectsProcessed++
			totalSize = totalSize + *item.Size
			if item.LastModified.After(lastModified) {
				lastModified = *item.LastModified
				lastObjectSize = *item.Size
			}
			if *item.Size > biggestObjectSize {
				biggestObjectSize = *item.Size
			}
		}

		// Production safety: break if we hit the limit
		if objectsProcessed >= int64(maxObjectsLimit) {
			log.Warnf("Hit production safety limit, processed %d objects", objectsProcessed)
			break
		}

		if resp.NextContinuationToken == nil {
			break
		}
		query.ContinuationToken = resp.NextContinuationToken
	}

	listDuration := time.Now().Sub(startList).Seconds()

	ch <- prometheus.MustNewConstMetric(
		s3ListSuccess, prometheus.GaugeValue, 1, e.bucket, e.prefix, e.delimiter,
	)
	ch <- prometheus.MustNewConstMetric(
		s3ListDuration, prometheus.GaugeValue, listDuration, e.bucket, e.prefix, e.delimiter,
	)
	if e.delimiter == "" {
		ch <- prometheus.MustNewConstMetric(
			s3LastModifiedObjectDate, prometheus.GaugeValue, float64(lastModified.UnixNano()/1e9), e.bucket, e.prefix,
		)
		ch <- prometheus.MustNewConstMetric(
			s3LastModifiedObjectSize, prometheus.GaugeValue, float64(lastObjectSize), e.bucket, e.prefix,
		)
		ch <- prometheus.MustNewConstMetric(
			s3ObjectTotal, prometheus.GaugeValue, numberOfObjects, e.bucket, e.prefix,
		)
		ch <- prometheus.MustNewConstMetric(
			s3SumSize, prometheus.GaugeValue, float64(totalSize), e.bucket, e.prefix,
		)
		ch <- prometheus.MustNewConstMetric(
			s3BiggestObjectSize, prometheus.GaugeValue, float64(biggestObjectSize), e.bucket, e.prefix,
		)
	}
	ch <- prometheus.MustNewConstMetric(
		s3CommonPrefixes, prometheus.GaugeValue, float64(commonPrefixes), e.bucket, e.prefix, e.delimiter,
	)
}

func probeHandler(w http.ResponseWriter, r *http.Request, svc s3iface.S3API) {
	bucket := r.URL.Query().Get("bucket")
	if bucket == "" {
		http.Error(w, "Bucket parameter is missing", 400)
		return
	}

	prefix := r.URL.Query().Get("prefix")
	delimiter := r.URL.Query().Get("delimiter")

	registry := prometheus.NewRegistry()
	exporter := &Exporter{
		svc:           svc,
		bucket:        bucket,
		prefix:        prefix,
		delimiter:     delimiter,
		useHeadMethod: useHeadMethod,
		prodSafeMode:  prodSafeMode,
	}
	registry.MustRegister(exporter)

	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
}

func discoveryHandler(w http.ResponseWriter, r *http.Request, svc s3iface.S3API) {
	buckets, err := svc.ListBuckets(&s3.ListBucketsInput{})
	if err != nil {
		http.Error(w, "Failed to list buckets", 500)
		return
	}

	type Target struct {
		Targets []string          `json:"targets"`
		Labels  map[string]string `json:"labels"`
	}

	var targets []Target
	for _, bucket := range buckets.Buckets {
		targets = append(targets, Target{
			Targets: []string{r.Host},
			Labels: map[string]string{
				"__param_bucket": *bucket.Name,
			},
		})
	}

	data, err := json.Marshal(targets)
	if err != nil {
		http.Error(w, "error marshalling json", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(data); err != nil {
		log.Errorln("failed to write JSON response:", err)
	}
}

func init() {
	prometheus.MustRegister(version.NewCollector(namespace + "_exporter"))
}

func main() {
	var (
		listenAddress  = kingpin.Flag("web.listen-address", "Address to listen on for web interface and telemetry.").Default(":9340").String()
		metricsPath    = kingpin.Flag("web.metrics-path", "Path under which to expose metrics").Default("/metrics").String()
		probePath      = kingpin.Flag("web.probe-path", "Path under which to expose the probe endpoint").Default("/probe").String()
		discoveryPath  = kingpin.Flag("web.discovery-path", "Path under which to expose service discovery").Default("/discovery").String()
		endpointURL    = kingpin.Flag("s3.endpoint-url", "Custom endpoint URL").String()
		disableSSL     = kingpin.Flag("s3.disable-ssl", "Custom disable SSL").Bool()
		forcePathStyle = kingpin.Flag("s3.force-path-style", "Custom force path style").Bool()
		useHeadFlag    = kingpin.Flag("s3.use-head-method", "Use HEAD method to get bucket usage from Ceph RGW").Bool()
		prodSafeFlag   = kingpin.Flag("s3.prod-safe-mode", "Production safe mode - never fall back to ListObjects (prevents OBJ incidents)").Bool()
		maxObjectsFlag = kingpin.Flag("s3.max-objects", "Maximum number of objects to process (production safety limit)").Default("10000").Int()
	)

	log.AddFlags(kingpin.CommandLine)
	kingpin.Version(version.Print("s3_exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	// Initialize global configuration
	useHeadMethod = *useHeadFlag
	prodSafeMode = *prodSafeFlag
	maxObjectsLimit = *maxObjectsFlag

	log.Infoln("Starting s3_exporter", version.Info())
	log.Infoln("Build context", version.BuildContext())
	log.Infof("Configuration: HEAD method=%v, Prod-safe mode=%v, Max objects=%d", useHeadMethod, prodSafeMode, maxObjectsLimit)

	config := &aws.Config{}
	if *endpointURL != "" {
		config.Endpoint = endpointURL
	}
	if *disableSSL {
		config.DisableSSL = disableSSL
	}
	if *forcePathStyle {
		config.S3ForcePathStyle = forcePathStyle
	}

	sess := session.Must(session.NewSession(config))
	if sess == nil {
		log.Errorln("Error creating sessions ", sess)
		os.Exit(1)
	}

	s3Client := s3.New(sess)

	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc(*probePath, func(w http.ResponseWriter, r *http.Request) {
		probeHandler(w, r, s3Client)
	})
	http.HandleFunc(*discoveryPath, func(w http.ResponseWriter, r *http.Request) {
		discoveryHandler(w, r, s3Client)
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte(`<html>
			<head><title>Linode Object Storage Exporter</title></head>
			<body>
			<h1>Linode Object Storage Exporter</h1>
			<p><a href="` + *probePath + `?bucket=BUCKET&prefix=PREFIX">Query metrics for objects in BUCKET that match PREFIX</a></p>
			<p><a href='` + *metricsPath + `'>Metrics</a></p>
			<p><a href='` + *discoveryPath + `'>Service Discovery</a></p>
			</body>
			</html>`))
		if err != nil {
			log.Errorln("failed to write response:", err)
		}
	})

	log.Infoln("Listening on", *listenAddress)
	srv := &http.Server{
		Addr:         *listenAddress,
		Handler:      nil,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}
