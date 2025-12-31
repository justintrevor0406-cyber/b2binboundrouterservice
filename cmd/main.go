package main

import (
	"crypto/tls"
	"os"
	"os/signal"
	"syscall"

	logger "github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
	"gitlab.dell.com/globalops/BPM/logistics/dragonfx-dao/golf_maverick/goservices/b2binboundrouterservice/internal/config"
	"gitlab.dell.com/globalops/BPM/logistics/dragonfx-dao/golf_maverick/goservices/b2binboundrouterservice/internal/handler"
	"gitlab.dell.com/globalops/BPM/logistics/dragonfx-dao/golf_maverick/goservices/b2binboundrouterservice/internal/orchestration"
	tconfig "gitlab.dell.com/globalops/BPM/logistics/dragonfx-dao/golf_maverick/goservices/toolkit/config"
	"gitlab.dell.com/globalops/BPM/logistics/dragonfx-dao/golf_maverick/goservices/toolkit/durabletask"
	"gitlab.dell.com/globalops/BPM/logistics/dragonfx-dao/golf_maverick/goservices/toolkit/rabbitmq"
	internalTLS "gitlab.dell.com/globalops/BPM/logistics/dragonfx-dao/golf_maverick/goservices/toolkit/tls"
)

var cfg config.Config

func main() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP)

	tconfig.ReloadOnSignal(sigCh, &cfg)

	err := tconfig.LoadConfig(&cfg)
	if err != nil {
		logger.Fatalf("Failed to load configuration: %v", err)
	}

    cfg.BackendConfig.PostgresOptionsSchema = cfg.PostgresOptionsSchema

	tlsConfig, err := internalTLS.NewTLSConfig()
	if err != nil {
		logger.Fatal(err)
	}

	amqpClient, err := rabbitmq.NewAMQPClient(cfg.GetRMQUrl(), tlsConfig,
		func(uri string, tlsConfig *tls.Config) (rabbitmq.AMQPConnection, error) {
			conn, err := amqp.DialTLS(uri, tlsConfig)
			if err != nil {
				return nil, err
			}

			return &rabbitmq.Connection{Connection: conn}, nil
		})
	if err != nil {
		logger.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}
	defer amqpClient.Close()

	activitiesByName, err := orchestration.NewDefaultActivitiesByNameFactory(&cfg, amqpClient).CreateActivitiesByName()
	if err != nil {
		logger.Fatalf("Failed to configure the activities: %v", err)
	}

	mainOrchestrator, err := orchestration.NewOrchestrator(activitiesByName)
	if err != nil {
		logger.Fatalf("Failed to configure the orchestrator: %v", err)
	}

	snOrchestrator, err := orchestration.NewSNOrchestrator(activitiesByName)
	if err != nil {
		logger.Fatalf("Failed to configure the orchestrator: %v", err)
	}

	obasnOrchestrator, err := orchestration.NewOBASNOrchestrator(activitiesByName)
	if err != nil {
		logger.Fatalf("Failed to configure the orchestrator: %v", err)
	}

	wossOrchestrator, err := orchestration.NewWOSSOrchestrator(activitiesByName)
	if err != nil {
		logger.Fatalf("Failed to configure the orchestrator: %v", err)
	}

	woAckOrchestrator, err := orchestration.NewWOAckOrchestrator(activitiesByName)
	if err != nil {
		logger.Fatalf("Failed to configure the orchestrator: %v", err)
	}
	woChangeackOrchestrator, err := orchestration.NewWOChangeAckOrchestrator(activitiesByName)
	if err != nil {
		logger.Fatalf("Failed to configure the orchestrator: %v", err)
	}
	ibasnAckNackOrchestrator, err := orchestration.NewIBASNAckOrchestrator(activitiesByName)
	if err != nil {
		logger.Fatalf("Failed to configure the orchestrator: %v", err)
	}
	ibasnReceiptOrchestrator, err := orchestration.NewIBASNReceiptOrchestrator(activitiesByName)
	if err != nil {
		logger.Fatalf("Failed to configure the orchestrator: %v", err)
	}

	nfOrchestrator, err := orchestration.NewNFOrchestrator(activitiesByName)
	if err != nil {
		logger.Fatalf("Failed to configure the orchestrator: %v", err)
	}

	crossDockOrchestrator, err := orchestration.NewCrossDockOrchestrator(activitiesByName)
	if err != nil {
		logger.Fatalf("Failed to configure the orchestrator: %v", err)
	}

	crossDockWoAckOrchestrator, err := orchestration.NewCrossDockWOAckOrchestrator(activitiesByName)
	if err != nil {
		logger.Fatalf("Failed to configure the orchestrator: %v", err)
	}
	reverseManifesOrchestrator, err := orchestration.NewReverseManifestOrchestrator(activitiesByName)
	if err != nil {
		logger.Fatalf("Failed to configure the orchestrator: %v", err)
	}

	orchestratorsByName := map[string]durabletask.Orchestrator{
		cfg.OrchestratorName:                         mainOrchestrator,
		orchestration.SNSubOrchestrator:              snOrchestrator,
		orchestration.OBASNSubOrchestrator:           obasnOrchestrator,
		orchestration.WOStatusSubOrchestrator:        wossOrchestrator,
		orchestration.WOAckSubOrchestrator:           woAckOrchestrator,
		orchestration.WOChangeAckSubOrchestrator:     woChangeackOrchestrator,
		orchestration.IBASNAckSubOrchestrator:        ibasnAckNackOrchestrator,
		orchestration.IBASNReceiptSubOrchestrator:    ibasnReceiptOrchestrator,
		orchestration.NFSubOrchestrator:              nfOrchestrator,
		orchestration.CrossDockSubOrchestrator:       crossDockOrchestrator,
		orchestration.ReverseManifestSubOrchestrator: reverseManifesOrchestrator,
		orchestration.CrossDockWoAckSubOrchestrator:  crossDockWoAckOrchestrator,
	}

	log := durabletask.NewDefaultLoggerFactory().CreateLogger()
	pv := durabletask.NewDefaultBackendProvider(cfg.BackendConfig, log).ProvideBackend()
	schedulingRequirement := durabletask.CreateMultipleOrchestratorsSchedulingRequirement(
		orchestratorsByName,
		*activitiesByName,
		*pv,
		*log,
	)

	defer durabletask.ShutdownTaskHubWorker(schedulingRequirement.Worker, schedulingRequirement.Ctx)

	ch, err := amqpClient.Consume(cfg.RmqB2BQueueName)
	if err != nil {
		logger.Fatalf("Failed to register consumer: %v", err)
	}
	//listener
	go func() {
		handler := handler.NewMessageHandler(&cfg, schedulingRequirement)
		handler.Handle(ch)
	}()

	logger.Info("Waiting for messages...")

	select {}
}
