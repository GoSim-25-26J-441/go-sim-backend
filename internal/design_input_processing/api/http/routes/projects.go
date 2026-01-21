package routes

import (
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/chats"
	dipdiagrams "github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/diagrams"
	dipllm "github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/llm"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProjectDeps struct {
	DB   *pgxpool.Pool
	UIGP *dipllm.UIGPClient
}

func RegisterProjectRoutes(projectsGroup *gin.RouterGroup, dep ProjectDeps) {
	// Diagrams
	diagramsRepo := dipdiagrams.NewRepo(dep.DB)
	dipdiagrams.RegisterProjectRoutes(projectsGroup, diagramsRepo)

	// Chats
	chatRepo := chats.NewRepo(dep.DB)
	chatHandler := chats.NewHandler(chatRepo, dep.UIGP)
	chats.RegisterProjectRoutes(projectsGroup, chatHandler)
}
