/*
Copyright 2019 The Ceph-CSI Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package liveness

import (
	"context"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	connlib "github.com/kubernetes-csi/csi-lib-utils/connection"
	"github.com/kubernetes-csi/csi-lib-utils/rpc"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog"
)

var (
	liveness = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "csi",
		Name:      "liveness",
		Help:      "Liveness Probe",
	})
)

func getLiveness(endpoint string, timeout time.Duration) {

	csiConn, err := connlib.Connect(endpoint)
	if err != nil {
		// connlib should retry forever so a returned error should mean
		// the grpc client is misconfigured rather than an error on the network
		klog.Fatalf("failed to establish connection to CSI driver: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	klog.Info("Sending probe request to CSI driver")
	ready, err := rpc.Probe(ctx, csiConn)
	if err != nil {
		liveness.Set(0)
		klog.Errorf("health check failed: %v", err)
		return
	}

	if !ready {
		liveness.Set(0)
		klog.Error("driver responded but is not ready")
		return
	}
	liveness.Set(1)
	klog.Infof("Health check succeeded")
}

func recordLiveness(endpoint string, pollTime, timeout time.Duration) {
	// register prometheus metrics
	err := prometheus.Register(liveness)
	if err != nil {
		klog.Fatalln(err)
	}

	// get liveness periodically
	ticker := time.NewTicker(pollTime)
	defer ticker.Stop()
	for range ticker.C {
		getLiveness(endpoint, timeout)
	}
}

func Run(endpoint, livenessendpoint string, port int, pollTime, timeout time.Duration) {
	klog.Infof("Liveness Running")

	ip := os.Getenv("POD_IP")

	if ip == "" {
		klog.Warning("missing POD_IP env var defaulting to 0.0.0.0")
		ip = "0.0.0.0"
	}

	// start liveness collection
	go recordLiveness(endpoint, pollTime, timeout)

	// start up prometheus endpoint
	addr := net.JoinHostPort(ip, strconv.Itoa(port))
	http.Handle(livenessendpoint, promhttp.Handler())
	err := http.ListenAndServe(addr, nil)
	if err != nil {
		klog.Fatalln(err)
	}
}