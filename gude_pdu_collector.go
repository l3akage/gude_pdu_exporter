package main

import (
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/soniah/gosnmp"
)

const prefix = "gude_pdu_"

var (
	upDesc            *prometheus.Desc
	totalDesc         *prometheus.Desc
	activePowerDesc   *prometheus.Desc
	actualCurrentDesc *prometheus.Desc
	actualVoltageDesc *prometheus.Desc
	frequencyDesc     *prometheus.Desc
)

func init() {
	l := []string{"target"}
	upDesc = prometheus.NewDesc(prefix+"up", "Scrape of target was successful", l, nil)
	l = append(l, "feed")
	totalDesc = prometheus.NewDesc(prefix+"total", "Total accumulated Active Energy of Power Channel", l, nil)
	activePowerDesc = prometheus.NewDesc(prefix+"active_power", "Active Power", l, nil)
	actualCurrentDesc = prometheus.NewDesc(prefix+"actual_current", "Actual Current on Power Channel", l, nil)
	actualVoltageDesc = prometheus.NewDesc(prefix+"actual_voltage", "Actual Voltage on Power Channel", l, nil)
	frequencyDesc = prometheus.NewDesc(prefix+"frequency", "Frequency of Power Channel", l, nil)
}

type gudePduCollector struct {
}

func (c gudePduCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- upDesc
	ch <- totalDesc
	ch <- activePowerDesc
	ch <- actualCurrentDesc
	ch <- actualVoltageDesc
	ch <- frequencyDesc
}

func (c gudePduCollector) collectMetrics(target, device string, ch chan<- prometheus.Metric, pdu gosnmp.SnmpPDU) error {
	name := pdu.Name[22:]
	feed := "A"
	if strings.HasSuffix(name, ".2") {
		feed = "B"
	}
	name = name[:len(name)-2]
	switch name {
	case "1.5.1.2.1.3":
		ch <- prometheus.MustNewConstMetric(totalDesc, prometheus.GaugeValue, float64(pdu.Value.(uint)), target, feed)
	case "1.5.1.2.1.4":
		ch <- prometheus.MustNewConstMetric(activePowerDesc, prometheus.GaugeValue, float64(pdu.Value.(int)), target, feed)
	case "1.5.1.2.1.5":
		ch <- prometheus.MustNewConstMetric(actualCurrentDesc, prometheus.GaugeValue, float64(pdu.Value.(uint)), target, feed)
	case "1.5.1.2.1.6":
		ch <- prometheus.MustNewConstMetric(actualVoltageDesc, prometheus.GaugeValue, float64(pdu.Value.(uint)), target, feed)
	case "1.5.1.2.1.7":
		ch <- prometheus.MustNewConstMetric(frequencyDesc, prometheus.GaugeValue, float64(pdu.Value.(uint)), target, feed)
	}
	return nil
}

func (c gudePduCollector) collectTarget(target string, ch chan<- prometheus.Metric, wg *sync.WaitGroup) {
	defer wg.Done()
	snmp := &gosnmp.GoSNMP{
		Target:    target,
		Port:      161,
		Community: *snmpCommunity,
		Version:   gosnmp.Version2c,
		Timeout:   time.Duration(2) * time.Second,
	}
	err := snmp.Connect()
	if err != nil {
		log.Infof("Connect() err: %v\n", err)
		ch <- prometheus.MustNewConstMetric(upDesc, prometheus.GaugeValue, 0, target)
		return
	}
	defer snmp.Conn.Close()

	oids := []string{"1.3.6.1.4.1.28507.27.1.1.1.1.0", "1.3.6.1.4.1.28507.38.1.1.1.1.0", "1.3.6.1.4.1.28507.62.1.1.1.1.0"}
	result, err2 := snmp.Get(oids)
	if err2 != nil {
		log.Infof("Get() err: %v\n", err)
		ch <- prometheus.MustNewConstMetric(upDesc, prometheus.GaugeValue, 0, target)
		return
	}
	device := ""
	for _, variable := range result.Variables {
		if variable.Value == nil {
			continue
		}
		switch variable.Name[1:] {
		case oids[0]:
			device = "27"
		case oids[1]:
			device = "38"
		case oids[2]:
			device = "62"
		}
	}
	err = snmp.Walk("1.3.6.1.4.1.28507."+device, func(pdu gosnmp.SnmpPDU) error {
		c.collectMetrics(target, device, ch, pdu)
		return nil
	})

	if err != nil {
		log.Infof("Walk() err: %v\n", err)
		ch <- prometheus.MustNewConstMetric(upDesc, prometheus.GaugeValue, 0, target)
		return
	}

	ch <- prometheus.MustNewConstMetric(upDesc, prometheus.GaugeValue, 1, target)
}

func (c gudePduCollector) Collect(ch chan<- prometheus.Metric) {
	targets := strings.Split(*snmpTargets, ",")
	wg := &sync.WaitGroup{}

	for _, target := range targets {
		wg.Add(1)
		go c.collectTarget(target, ch, wg)
	}

	wg.Wait()
}
