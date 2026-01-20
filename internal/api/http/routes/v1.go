package routes

import (
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/auth"
	diproutes "github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/api/http/routes"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/chats"
	dipdiagrams "github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/diagrams"
	dipllm "github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/llm"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/projects"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/users"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type V1Deps struct {
	DB          *pgxpool.Pool
	UpstreamURL string
	OllamaURL   string
}

func RegisterV1(r *gin.Engine, dep V1Deps) {
	api := r.Group("/api/v1")

	userRepo := users.NewRepo(dep.DB)
	api.Use(auth.WithUser(userRepo))

	projectRepo := projects.NewRepo(dep.DB)
	projectsGroup := api.Group("/projects")
	projects.Register(projectsGroup, projectRepo)

	chatRepo := chats.NewRepo(dep.DB)
	chatHandler := chats.NewHandler(chatRepo, dipllm.NewUIGP())

	chats.RegisterProjectChatRoutes(projectsGroup, chatHandler)

	diagramsRepo := dipdiagrams.NewRepo(dep.DB)
	dipdiagrams.RegisterProjectDiagramRoutes(projectsGroup, diagramsRepo)

	diproutes.RegisterV1(api, diproutes.V1Deps{
		UpstreamURL: dep.UpstreamURL,
		OllamaURL:   dep.OllamaURL,
	})
}
