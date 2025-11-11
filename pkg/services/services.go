package services

import (
	"fmt"
	"sync"
	"time"

	"github.com/velvee-ai/ai-workflow/pkg/config"
	"github.com/velvee-ai/ai-workflow/pkg/gitexec"
)

// Services holds all application-wide singleton services.
type Services struct {
	Config    *config.Config
	GitRunner *gitexec.Runner
	// Future: WorktreeManager, CacheService, IDEOpener, GitHubClient, etc.
}

var (
	instance *Services
	once     sync.Once
	initErr  error
)

// Init initializes the services singleton. Call this during app initialization.
func Init() error {
	once.Do(func() {
		cfg, err := config.Get()
		if err != nil {
			initErr = fmt.Errorf("failed to load config: %w", err)
			return
		}

		// Initialize git runner with timeout
		gitRunner := gitexec.New(30 * time.Second)

		instance = &Services{
			Config:    cfg,
			GitRunner: gitRunner,
		}
	})

	return initErr
}

// MustInit initializes services and panics on error. Use for startup scenarios.
func MustInit() {
	if err := Init(); err != nil {
		panic(fmt.Sprintf("failed to initialize services: %v", err))
	}
}

// Get returns the services singleton. Must call Init() or MustInit() first.
func Get() *Services {
	if instance == nil {
		panic("services not initialized; call services.Init() first")
	}
	return instance
}

// Reset clears the singleton for testing purposes.
func Reset() {
	instance = nil
	once = sync.Once{}
	initErr = nil
}
