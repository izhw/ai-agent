package main

import (
	"context"
	"flag"
	"log"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/izhw/ai-agent/adapter/eino"
	"github.com/izhw/ai-agent/adapter/memory/portable"
	modeladapter "github.com/izhw/ai-agent/adapter/model"
	secretenv "github.com/izhw/ai-agent/adapter/secrets/env"
	"github.com/izhw/ai-agent/adapter/storage/gormstore"
	memoryadapter "github.com/izhw/ai-agent/adapter/storage/memory"
	"github.com/izhw/ai-agent/agentkit"
	"github.com/izhw/ai-agent/service/httpapi"
	runtimespi "github.com/izhw/ai-agent/spi/runtime"
	storagespi "github.com/izhw/ai-agent/spi/storage"
)

func main() {
	configPath := flag.String("config", "configs/config.example.yaml", "configuration file")
	flag.Parse()
	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	store, err := newStorage(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	models := eino.NewModelRegistry()
	secretResolver := secretenv.Resolver{}
	modelFactory := modeladapter.NewFactory()
	for ref, item := range cfg.Models {
		chatModel, modelErr := modelFactory.Build(
			ctx,
			modeladapter.Config{
				Provider:      item.Provider,
				Model:         item.Model,
				BaseURL:       item.BaseURL,
				APIVersion:    item.APIVersion,
				CredentialRef: item.CredentialRef,
				Timeout:       item.Timeout,
				Temperature:   item.Temperature,
				MaxTokens:     item.MaxTokens,
			},
			secretResolver,
		)
		if modelErr != nil {
			log.Fatal(modelErr)
		}
		if err = models.Register(ref, chatModel); err != nil {
			log.Fatal(err)
		}
	}
	presets := make(map[string]eino.DeepPreset, len(cfg.DeepPresets))
	for ref, item := range cfg.DeepPresets {
		presets[ref] = eino.DeepPreset{
			MaxIterations:          item.MaxIterations,
			WithoutWriteTodos:      item.WithoutWriteTodos,
			WithoutGeneralSubAgent: item.WithoutGeneralSubAgent,
		}
	}
	deps := eino.Dependencies{
		Models:        models,
		Checkpoints:   store.Checkpoints(),
		CheckpointTTL: 24 * time.Hour,
		DeepPresets:   presets,
	}
	runtimes, err := runtimespi.NewRegistry(
		eino.NewChatModelFactory(deps),
		eino.NewDeepAgentFactory(deps),
	)
	if err != nil {
		log.Fatal(err)
	}
	memoryService := portable.NewService(store.Memories(), nil)
	engine, err := agentkit.New(
		agentkit.WithStorage(store),
		agentkit.WithRuntimes(runtimes),
		agentkit.WithMemory(memoryService),
	)
	if err != nil {
		log.Fatal(err)
	}
	h := server.Default(server.WithHostPorts(cfg.Address))
	httpapi.New(engine).Register(h)
	h.Spin()
}

func newStorage(ctx context.Context, cfg config) (storagespi.Storage, error) {
	switch strings.ToLower(cfg.Storage.Driver) {
	case "memory":
		return memoryadapter.New(), nil
	case "mysql", "postgres":
		var db *gorm.DB
		var err error
		var dialect gormstore.Dialect
		if cfg.Storage.Driver == "mysql" {
			db, err = gorm.Open(mysql.Open(cfg.Storage.DSN))
			dialect = gormstore.DialectMySQL
		} else {
			db, err = gorm.Open(postgres.Open(cfg.Storage.DSN))
			dialect = gormstore.DialectPostgres
		}
		if err != nil {
			return nil, err
		}
		store, err := gormstore.New(db, gormstore.WithDialect(dialect))
		if err != nil {
			return nil, err
		}
		if cfg.Storage.AutoMigrate {
			if err = store.Migrate(ctx); err != nil {
				return nil, err
			}
		}
		return store, nil
	default:
		return nil, &unsupportedStorageError{driver: cfg.Storage.Driver}
	}
}

type unsupportedStorageError struct{ driver string }

func (e *unsupportedStorageError) Error() string {
	return "unsupported storage driver: " + e.driver + "; inject a custom Storage for Dameng or other databases"
}
