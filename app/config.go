package app

import (
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type App struct {
	serverAddr         string
	listenAddr         string
	prometheusCounters map[string]*prometheus.CounterVec
	prometheusGauges   map[string]*prometheus.GaugeVec
}

func Run(serverAddr string, listenAddr string) error {
	app := &App{
		serverAddr:         serverAddr,
		listenAddr:         listenAddr,
		prometheusCounters: make(map[string]*prometheus.CounterVec),
		prometheusGauges:   make(map[string]*prometheus.GaugeVec),
	}
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	return app.StartVegaObserver()
}
