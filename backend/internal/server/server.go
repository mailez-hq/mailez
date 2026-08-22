package server

import (
	"context"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/redis/go-redis/v9"
	glebarezsqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
	"mailess/backend/internal/api"
	"mailess/backend/internal/auth"
	"mailess/backend/internal/config"
	"mailess/backend/internal/internalapi"
	"mailess/backend/internal/models"
)

// Server bundles the Fiber app and its dependencies.
type Server struct {
	App     *fiber.App
	DB      *gorm.DB
	Redis   *redis.Client
	Cfg     config.Config
	Auth    *auth.Manager
	internal *internalapi.Handler
}

// New builds the Fiber app and initializes connections.
func New(cfg config.Config) *Server {
	db := connectDB(cfg)
	rdb := connectRedis(cfg)

	app := fiber.New(fiber.Config{
		AppName: "mailess",
	})
	app.Use(recover.New())
	app.Use(cors.New())

	s := &Server{App: app, DB: db, Redis: rdb, Cfg: cfg}
	s.Auth = auth.NewManager(db, newStore(rdb), "mailess_session", time.Duration(cfg.SessionLifetime)*time.Second)
	s.internal = internalapi.New(db, s.Auth, cfg)
	s.routes()
	return s
}

// newStore prefers Redis and falls back to memory for local dev.
func newStore(rdb *redis.Client) auth.Store {
	if err := rdb.Ping(context.Background()).Err(); err == nil {
		return auth.NewRedisStore(rdb)
	}
	return auth.NewMemoryStore()
}

func (s *Server) routes() {
	v1 := s.App.Group("/api/v1")
	v1.Get("/health", s.health)
	s.Auth.RegisterSSO(v1)
	apiHandler := api.New(s.DB, s.Auth, s.Cfg)
	apiHandler.Register(v1.Group(""))
	s.internal.Register(s.App.Group("/internal"))
}

func (s *Server) health(c *fiber.Ctx) error {
	sqlDB, _ := s.DB.DB()
	if err := sqlDB.Ping(); err != nil {
		return c.Status(500).JSON(fiber.Map{"status": "error", "db": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "ok"})
}

func connectDB(cfg config.Config) *gorm.DB {
	// Pure-Go sqlite driver (no cgo) for local dev; mysql driver lands with phase 1.
	// SingularTable keeps table names aligned with Mailu's schema for migration.
	db, err := gorm.Open(glebarezsqlite.Open(cfg.DBDSN), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	if err := models.AutoMigrate(db); err != nil {
		log.Fatalf("db migrate: %v", err)
	}
	return db
}

func connectRedis(cfg config.Config) *redis.Client {
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		// Redis is not critical for the scaffold; log and continue.
		log.Printf("redis connect: %v", err)
	}
	return rdb
}
