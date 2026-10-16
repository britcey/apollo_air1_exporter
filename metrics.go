package main

import (
	"bytes"
	"math"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

type metrics struct {
	registry *prometheus.Registry
	deviceUp *prometheus.GaugeVec
	sensors  map[string]*prometheus.GaugeVec
}

var vocQualityStates = []string{"Improved", "Normal", "Abnormal", "Very abnormal", "Extremely abnormal", "Unknown"}

func newMetrics() *metrics {
	registry := prometheus.NewRegistry()
	gauge := func(name, help string, labels ...string) *prometheus.GaugeVec {
		metric := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "apollo_air1_" + name, Help: help}, labels)
		registry.MustRegister(metric)
		return metric
	}
	result := &metrics{
		registry: registry,
		deviceUp: gauge("device_up", "Whether the Apollo Air-1 device is reachable (1) or not (0)", "device", "host"),
		sensors:  make(map[string]*prometheus.GaugeVec),
	}
	for _, sensor := range sensors {
		labels := []string{"device", "host"}
		if sensor.domain == "text_sensor" {
			if sensor.id == "voc_quality" {
				labels = append(labels, "state")
			} else {
				labels = append(labels, "value")
			}
		}
		metric := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "apollo_air1_" + sensor.metric,
			Help:        sensor.help,
			ConstLabels: prometheus.Labels{"sensor": sensor.name},
		}, labels)
		registry.MustRegister(metric)
		result.sensors[sensor.id] = metric
	}
	return result
}

func (metrics *metrics) updateDevice(device device, values map[string]any) {
	metrics.deviceUp.WithLabelValues(device.name, device.host).Set(1)
	for id, reading := range values {
		metric, ok := metrics.sensors[id]
		if !ok {
			continue
		}
		switch value := reading.(type) {
		case float64:
			if id == "rssi" {
				value = math.Trunc(value)
			}
			metric.WithLabelValues(device.name, device.host).Set(value)
		case bool:
			number := 0.0
			if value {
				number = 1
			}
			metric.WithLabelValues(device.name, device.host).Set(number)
		case string:
			if id == "voc_quality" {
				current := "Unknown"
				for _, state := range vocQualityStates {
					if value == state {
						current = state
						break
					}
				}
				for _, state := range vocQualityStates {
					number := 0.0
					if state == current {
						number = 1
					}
					metric.WithLabelValues(device.name, device.host, state).Set(number)
				}
			} else {
				metric.DeletePartialMatch(prometheus.Labels{"device": device.name, "host": device.host})
				metric.WithLabelValues(device.name, device.host, value).Set(1)
			}
		}
	}
}

func (metrics *metrics) markDeviceDown(device device) {
	metrics.deviceUp.WithLabelValues(device.name, device.host).Set(0)
}

func (metrics *metrics) gather() (string, error) {
	families, err := metrics.registry.Gather()
	if err != nil {
		return "", err
	}
	var output bytes.Buffer
	for _, family := range families {
		if _, err := expfmt.MetricFamilyToText(&output, family); err != nil {
			return "", err
		}
	}
	return output.String(), nil
}
